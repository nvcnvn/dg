package ops

import (
	"bytes"
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/view"
)

// Files are written from their tokens as they are: hclwrite's File.Bytes formats the whole file,
// so every edit here spaces the tokens it creates itself and leaves every other token alone.

func source(f *hclwrite.File) []byte {
	return f.BuildTokens(nil).Bytes()
}

func parse(name string, src []byte) (*hclwrite.File, error) {
	f, diags := hclwrite.ParseConfig(src, name, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s does not parse: %s", name, diags.Error())
	}
	return f, nil
}

// ref returns the traversal root.names..., each name in index form when it is not simple.
func ref(root string, names ...string) hcl.Traversal {
	tr := hcl.Traversal{hcl.TraverseRoot{Name: root}}
	for _, n := range names {
		tr = append(tr, step(n))
	}
	return tr
}

func step(name string) hcl.Traverser {
	if spec.IsSimpleID(name) {
		return hcl.TraverseAttr{Name: name}
	}
	return hcl.TraverseIndex{Key: cty.StringVal(name)}
}

// endRef is the traversal of an edge end.
func endRef(e view.End) hcl.Traversal {
	switch {
	case e.Port != "":
		return ref("node", e.Node, "port", e.Port)
	case e.Row != "":
		return ref("node", e.Node, "row", e.Row)
	}
	return ref("node", e.Node)
}

// lineStarts calls f for every token that starts a line of toks, the first included.
func lineStarts(toks hclwrite.Tokens, f func(*hclwrite.Token)) {
	start := true
	for _, t := range toks {
		if start && t.Type != hclsyntax.TokenNewline {
			f(t)
		}
		start = t.Type == hclsyntax.TokenNewline || t.Type == hclsyntax.TokenComment && bytes.HasSuffix(t.Bytes, []byte("\n"))
	}
}

// newBlock returns a parsed copy of b in canonical format for a body indented by indent spaces,
// with empty bodies written {}.
func newBlock(b *hclwrite.Block, indent int) *hclwrite.Block {
	tmp := hclwrite.NewEmptyFile()
	outer := tmp.Body().AppendNewBlock("x", nil)
	outer.Body().AppendBlock(b)
	toks := tmp.BuildTokens(nil)
	var out hclwrite.Tokens
	for i, t := range toks {
		if t.Type == hclsyntax.TokenNewline && i > 0 && i+1 < len(toks) && toks[i-1].Type == hclsyntax.TokenOBrace && toks[i+1].Type == hclsyntax.TokenCBrace {
			continue
		}
		out = append(out, t)
	}
	f, diags := hclwrite.ParseConfig(hclwrite.Format(out.Bytes()), "", hcl.InitialPos)
	if diags.HasErrors() {
		panic(fmt.Sprintf("parsing a block built in memory: %v", diags))
	}
	inner := f.Body().Blocks()[0].Body().Blocks()[0]
	lineStarts(inner.BuildTokens(nil), func(t *hclwrite.Token) { t.SpacesBefore += indent - 2 })
	return inner
}

// appendBlock appends b to the end of f after a blank line.
func appendBlock(f *hclwrite.File, b *hclwrite.Block) {
	if src := source(f); len(bytes.TrimSpace(src)) > 0 {
		if !bytes.HasSuffix(src, []byte("\n")) {
			f.Body().AppendNewline()
		}
		if !bytes.HasSuffix(src, []byte("\n\n")) {
			f.Body().AppendNewline()
		}
	}
	f.Body().AppendBlock(newBlock(b, 0))
}

// appendNested appends b to the end of body.
func appendNested(body *hclwrite.Body, b *hclwrite.Block) {
	body.AppendBlock(newBlock(b, indentOf(body)))
}

// indentOf returns the indentation of body's lines, or 2.
func indentOf(body *hclwrite.Body) int {
	indent := -1
	lineStarts(body.BuildTokens(nil), func(t *hclwrite.Token) {
		if indent < 0 && t.Type != hclsyntax.TokenCBrace {
			indent = t.SpacesBefore
		}
	})
	if indent < 0 {
		return 2
	}
	return indent
}

// exprTokens returns the tokens of expression toks in canonical format, as the parser makes them.
func exprTokens(toks hclwrite.Tokens) hclwrite.Tokens {
	src := append(append([]byte("x = "), toks.Bytes()...), '\n')
	f, diags := hclwrite.ParseConfig(hclwrite.Format(src), "", hcl.InitialPos)
	if diags.HasErrors() {
		panic(fmt.Sprintf("parsing an expression built in memory: %v", diags))
	}
	return f.Body().GetAttribute("x").Expr().BuildTokens(nil)
}

// setAttr sets attribute name of body to the expression toks. A new attribute is indented like
// body. The = signs and trailing comments of the run of attributes it is in are then aligned as
// hclwrite.Format aligns them.
func setAttr(body *hclwrite.Body, name string, toks hclwrite.Tokens) {
	attr := body.GetAttribute(name)
	isNew := attr == nil
	indent := indentOf(body)
	if !isNew {
		at := attr.BuildTokens(nil)
		indent = at[slices.IndexFunc(at, func(t *hclwrite.Token) bool { return t.Type == hclsyntax.TokenIdent })].SpacesBefore
	}
	expr := exprTokens(toks)
	first := true
	lineStarts(expr, func(t *hclwrite.Token) {
		if !first {
			t.SpacesBefore += indent
		}
		first = false
	})
	body.SetAttributeRaw(name, expr)
	if isNew {
		at := body.GetAttribute(name).BuildTokens(nil)
		at[0].SpacesBefore, at[1].SpacesBefore = indent, 1
	}
	alignAttr(body, name)
}

// formatLine is a line of tokens split as hclwrite.Format splits it: up to the =, the = and
// a value on that line, and a trailing comment.
type formatLine struct {
	lead, assign, comment hclwrite.Tokens
}

func isLineEnd(t *hclwrite.Token) bool {
	return t.Type == hclsyntax.TokenNewline || t.Type == hclsyntax.TokenComment && bytes.HasSuffix(t.Bytes, []byte("\n"))
}

func bracketChange(t *hclwrite.Token) int {
	switch t.Type {
	case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen, hclsyntax.TokenTemplateControl, hclsyntax.TokenTemplateInterp:
		return 1
	case hclsyntax.TokenCBrace, hclsyntax.TokenCBrack, hclsyntax.TokenCParen, hclsyntax.TokenTemplateSeqEnd:
		return -1
	}
	return 0
}

func formatLines(toks hclwrite.Tokens) []formatLine {
	var lines []formatLine
	start := 0
	for i, t := range toks {
		if isLineEnd(t) || i == len(toks)-1 {
			lines = append(lines, formatLine{lead: toks[start : i+1]})
			start = i + 1
		}
	}
	for i := range lines {
		l := &lines[i]
		if len(l.lead) > 1 && l.lead[len(l.lead)-1].Type == hclsyntax.TokenComment {
			l.comment, l.lead = l.lead[len(l.lead)-1:], l.lead[:len(l.lead)-1]
		}
		for j, t := range l.lead {
			if j == 0 || t.Type != hclsyntax.TokenEqual {
				continue
			}
			net := 0
			for _, u := range l.lead[j:] {
				net += bracketChange(u)
			}
			if net == 0 {
				l.assign, l.lead = l.lead[j:], l.lead[:j]
			}
			break
		}
	}
	return lines
}

// alignAttr aligns, as hclwrite.Format does, the = signs of the run of one-line attributes that
// attribute name of body is in, and the trailing comments of the runs of commented lines that
// touch that run or the attribute's lines; every other line stays as it is.
func alignAttr(body *hclwrite.Body, name string) {
	at := body.GetAttribute(name).BuildTokens(nil)
	lines := formatLines(body.BuildTokens(nil))
	holds := func(t *hclwrite.Token) func(formatLine) bool {
		return func(l formatLine) bool { return slices.Contains(l.lead, t) || slices.Contains(l.assign, t) }
	}
	first, last := slices.IndexFunc(lines, holds(at[0])), slices.IndexFunc(lines, holds(at[len(at)-1]))
	if first < 0 || last < 0 {
		return
	}
	run := func(at int, in func(formatLine) bool) (int, int) {
		s, e := at, at+1
		for s > 0 && in(lines[s-1]) {
			s--
		}
		for e < len(lines) && in(lines[e]) {
			e++
		}
		return s, e
	}
	s, e := first, last+1
	if lines[first].assign != nil {
		s, e = run(first, func(l formatLine) bool { return l.assign != nil })
		widest := 0
		for _, l := range lines[s:e] {
			widest = max(widest, l.lead.Columns())
		}
		for _, l := range lines[s:e] {
			l.assign[0].SpacesBefore = widest - l.lead.Columns() + 1
		}
		e = max(e, last+1)
	}
	for k := s; k < e; k++ {
		if lines[k].comment == nil {
			continue
		}
		cs, ce := run(k, func(l formatLine) bool { return l.comment != nil })
		widest := 0
		for _, l := range lines[cs:ce] {
			widest = max(widest, l.lead.Columns()+l.assign.Columns())
		}
		for _, l := range lines[cs:ce] {
			l.comment[0].SpacesBefore = widest - l.lead.Columns() - l.assign.Columns() + 1
		}
		k = ce - 1
	}
}

// removeAttr removes attribute name of body and aligns the attributes it was between.
func removeAttr(body *hclwrite.Body, name string) {
	attr := body.GetAttribute(name)
	if attr == nil {
		return
	}
	lines := formatLines(body.BuildTokens(nil))
	at := attr.BuildTokens(nil)
	var neighbors []string
	for i, l := range lines {
		if !slices.Contains(l.lead, at[0]) {
			continue
		}
		for _, j := range []int{i - 1, i + 1} {
			if j >= 0 && j < len(lines) && len(lines[j].lead) > 0 && lines[j].assign != nil {
				if n := string(lines[j].lead[0].Bytes); n != name && body.GetAttribute(n) != nil {
					neighbors = append(neighbors, n)
				}
			}
		}
	}
	body.RemoveAttribute(name)
	for _, n := range neighbors {
		alignAttr(body, n)
	}
}

func setValue(body *hclwrite.Body, name string, v cty.Value) {
	setAttr(body, name, hclwrite.TokensForValue(v))
}

func setTraversal(body *hclwrite.Body, name string, tr hcl.Traversal) {
	setAttr(body, name, hclwrite.TokensForTraversal(tr))
}

// setLabel renames block b, keeping the space before its label.
func setLabel(b *hclwrite.Block, label string) {
	spaces := labelToken(b).SpacesBefore
	b.SetLabels([]string{label})
	labelToken(b).SpacesBefore = spaces
}

// labelToken returns the first token of b's label, after the comments before b and its type.
func labelToken(b *hclwrite.Block) *hclwrite.Token {
	toks := b.BuildTokens(nil)
	i := slices.IndexFunc(toks, func(t *hclwrite.Token) bool { return t.Type == hclsyntax.TokenIdent })
	return toks[i+1]
}

// blocks returns the blocks of body with type typ.
func blocks(body *hclwrite.Body, typ string) []*hclwrite.Block {
	return slices.DeleteFunc(slices.Clone(body.Blocks()), func(b *hclwrite.Block) bool { return b.Type() != typ })
}

// sortedAttrs returns body's attributes sorted by name.
func sortedAttrs(body *hclwrite.Body) []string {
	return slices.Sorted(maps.Keys(body.Attributes()))
}

func tok(typ hclsyntax.TokenType, s string) *hclwrite.Token {
	return &hclwrite.Token{Type: typ, Bytes: []byte(s)}
}

// stepTokens returns the tokens of a traversal step naming id: .id or ["id"].
func stepTokens(id string) hclwrite.Tokens {
	if spec.IsSimpleID(id) {
		return hclwrite.Tokens{tok(hclsyntax.TokenDot, "."), tok(hclsyntax.TokenIdent, id)}
	}
	quoted := hclwrite.TokensForValue(cty.StringVal(id))
	return append(append(hclwrite.Tokens{tok(hclsyntax.TokenOBrack, "[")}, quoted...), tok(hclsyntax.TokenCBrack, "]"))
}

// renameTokens replaces, in the tokens of an expression, the step after each root in roots that
// names old with one naming id. A reference written another way (an unusual escape) is left as
// it is; validation then refuses the operation.
func renameTokens(toks hclwrite.Tokens, roots []string, old, id string) (hclwrite.Tokens, bool) {
	var out hclwrite.Tokens
	changed := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		out = append(out, t)
		if t.Type != hclsyntax.TokenIdent || !slices.Contains(roots, string(t.Bytes)) || i > 0 && toks[i-1].Type == hclsyntax.TokenDot {
			continue
		}
		if name, n := stepAt(toks[i+1:]); n > 0 && name == old {
			out = append(out, stepTokens(id)...)
			i += n
			changed = true
		}
	}
	return out, changed
}

// stepAt returns the name a traversal step at the start of toks names, .name or ["name"], and
// how many tokens it takes (0 when toks does not start with a step).
func stepAt(toks hclwrite.Tokens) (string, int) {
	if len(toks) >= 2 && toks[0].Type == hclsyntax.TokenDot && toks[1].Type == hclsyntax.TokenIdent {
		return string(toks[1].Bytes), 2
	}
	if len(toks) < 4 || toks[0].Type != hclsyntax.TokenOBrack || toks[1].Type != hclsyntax.TokenOQuote {
		return "", 0
	}
	var lit []byte
	for j := 2; j+1 < len(toks); j++ {
		switch toks[j].Type {
		case hclsyntax.TokenQuotedLit:
			lit = append(lit, toks[j].Bytes...)
			continue
		case hclsyntax.TokenCQuote:
			if toks[j+1].Type != hclsyntax.TokenCBrack {
				return "", 0
			}
			s, diags := hclsyntax.ParseStringLiteralToken(hclsyntax.Token{Type: hclsyntax.TokenQuotedLit, Bytes: lit})
			if diags.HasErrors() {
				return "", 0
			}
			return s, j + 2
		}
		return "", 0
	}
	return "", 0
}

// renameIn renames references in every attribute of body and its nested blocks, changing no
// other token.
func renameIn(body *hclwrite.Body, roots []string, old, id string) bool {
	changed := false
	for _, name := range sortedAttrs(body) {
		toks := body.GetAttribute(name).Expr().BuildTokens(nil)
		if out, ok := renameTokens(toks, roots, old, id); ok {
			body.SetAttributeRaw(name, out)
			alignAttr(body, name)
			changed = true
		}
	}
	for _, b := range body.Blocks() {
		changed = renameIn(b.Body(), roots, old, id) || changed
	}
	return changed
}

// list is a list literal split into its items, keeping its own tokens.
type list struct {
	open  hclwrite.Tokens // [ and the comments on its line
	items []listItem
	tail  hclwrite.Tokens // newlines and comments before ]
	close *hclwrite.Token
}

type listItem struct {
	lead    hclwrite.Tokens // newlines and comments before the item
	content hclwrite.Tokens
	comma   *hclwrite.Token // the comma after the item, if any
	trail   hclwrite.Tokens // comments after the comma on the item's line
}

func isSpace(t *hclwrite.Token) bool {
	return t.Type == hclsyntax.TokenNewline || t.Type == hclsyntax.TokenComment
}

// sameLine returns the comments at the start of toks that are on the line before its first newline.
func sameLine(toks hclwrite.Tokens) int {
	n := 0
	for n < len(toks) && toks[n].Type == hclsyntax.TokenComment {
		n++
		if bytes.HasSuffix(toks[n-1].Bytes, []byte("\n")) {
			break
		}
	}
	return n
}

// splitList splits the tokens of a list literal; ok is false for any other expression.
func splitList(toks hclwrite.Tokens) (l list, ok bool) {
	if len(toks) < 2 || toks[0].Type != hclsyntax.TokenOBrack || toks[len(toks)-1].Type != hclsyntax.TokenCBrack {
		return l, false
	}
	l.open, l.close = toks[:1], toks[len(toks)-1]
	var segs []hclwrite.Tokens
	var commas []*hclwrite.Token
	var cur hclwrite.Tokens
	depth := 0
	for _, t := range toks[1 : len(toks)-1] {
		switch t.Type {
		case hclsyntax.TokenOBrack, hclsyntax.TokenOParen, hclsyntax.TokenOBrace, hclsyntax.TokenTemplateInterp:
			depth++
		case hclsyntax.TokenCBrack, hclsyntax.TokenCParen, hclsyntax.TokenCBrace, hclsyntax.TokenTemplateSeqEnd:
			depth--
		}
		if t.Type == hclsyntax.TokenComma && depth == 0 {
			segs, commas, cur = append(segs, cur), append(commas, t), nil
			continue
		}
		cur = append(cur, t)
	}
	if depth != 0 {
		return l, false
	}
	segs = append(segs, cur)
	n := sameLine(segs[0])
	l.open = append(l.open, segs[0][:n]...)
	segs[0] = segs[0][n:]
	last := segs[len(segs)-1]
	if !slices.ContainsFunc(last, func(t *hclwrite.Token) bool { return !isSpace(t) }) {
		l.tail = last
		segs = segs[:len(segs)-1]
	} else {
		end := len(last)
		for end > 0 && isSpace(last[end-1]) {
			end--
		}
		l.tail = last[end:]
		segs[len(segs)-1] = last[:end]
	}
	for i, s := range segs {
		start := 0
		for start < len(s) && isSpace(s[start]) {
			start++
		}
		if start == len(s) {
			return l, false
		}
		it := listItem{lead: s[:start], content: s[start:]}
		if i < len(commas) {
			it.comma = commas[i]
		}
		l.items = append(l.items, it)
	}
	for i := range l.items {
		next := &l.tail
		if i+1 < len(l.items) {
			next = &l.items[i+1].lead
		}
		if l.items[i].comma != nil {
			n := sameLine(*next)
			l.items[i].trail, *next = (*next)[:n], (*next)[n:]
		}
	}
	return l, true
}

func (l list) tokens() hclwrite.Tokens {
	out := slices.Clone(l.open)
	for i, it := range l.items {
		out = append(out, it.lead...)
		out = append(out, it.content...)
		comma := it.comma
		if comma == nil && i+1 < len(l.items) {
			comma = tok(hclsyntax.TokenComma, ",")
		}
		if comma != nil {
			out = append(out, comma)
		}
		out = append(out, it.trail...)
	}
	return append(append(out, l.tail...), l.close)
}

func (l list) multiline() bool {
	return slices.ContainsFunc(l.tail, isSpace) || slices.ContainsFunc(l.items, func(it listItem) bool { return len(it.lead) > 0 })
}

// add appends an item, in the list's style: on its own line, indented like the item before, or
// after a comma and a space.
func (l *list) add(content hclwrite.Tokens) {
	it := listItem{content: content}
	if l.multiline() {
		indent := 2
		if n := len(l.items); n > 0 {
			indent = l.items[n-1].content[0].SpacesBefore
		}
		it.lead = hclwrite.Tokens{tok(hclsyntax.TokenNewline, "\n")}
		content[0].SpacesBefore = indent
		if len(l.tail) == 0 {
			l.tail = hclwrite.Tokens{tok(hclsyntax.TokenNewline, "\n")}
			l.close.SpacesBefore = max(0, indent-2)
		}
		if n := len(l.items); n > 0 && l.items[n-1].comma != nil {
			it.comma = tok(hclsyntax.TokenComma, ",")
		}
	} else if len(l.items) > 0 {
		content[0].SpacesBefore = 1
		if n := len(l.items); l.items[n-1].comma != nil {
			l.items[n-1].comma = nil
			it.comma = tok(hclsyntax.TokenComma, ",")
		}
	}
	l.items = append(l.items, it)
}

// remove removes the items drop matches, with their comments on their line.
func (l *list) remove(drop func(listItem) bool) bool {
	var kept []listItem
	for _, it := range l.items {
		if !drop(it) {
			kept = append(kept, it)
		}
	}
	if len(kept) == len(l.items) {
		return false
	}
	if len(kept) > 0 && drop(l.items[0]) && len(kept[0].lead) == 0 {
		kept[0].content[0].SpacesBefore = l.items[0].content[0].SpacesBefore
	}
	if len(kept) > 0 && l.items[len(l.items)-1].comma == nil {
		kept[len(kept)-1].comma = nil
	}
	l.items = kept
	return true
}

// selector parses the selector an item of a list attribute holds.
func (it listItem) selector(edges bool) (view.Selector, bool) {
	expr, diags := hclsyntax.ParseExpression(it.content.Bytes(), "", hcl.InitialPos)
	if diags.HasErrors() {
		return view.Selector{}, false
	}
	s, diags := view.ParseSelector(expr, edges)
	return s, !diags.HasErrors()
}

// dropSelectors removes from the list attribute name of body the selectors drop matches. It
// reports whether it changed anything.
func dropSelectors(body *hclwrite.Body, name string, drop func(view.Selector) bool) bool {
	attr := body.GetAttribute(name)
	if attr == nil {
		return false
	}
	l, ok := splitList(attr.Expr().BuildTokens(nil))
	if !ok {
		return false
	}
	if !l.remove(func(it listItem) bool {
		s, ok := it.selector(name == "edges")
		return ok && drop(s)
	}) {
		return false
	}
	body.SetAttributeRaw(name, l.tokens())
	alignAttr(body, name)
	return true
}

// addSelector appends the selector sel to the list attribute name of body, which is written
// with def as its items when it is not set.
func addSelector(body *hclwrite.Body, name string, def []hcl.Traversal, sel hcl.Traversal) error {
	attr := body.GetAttribute(name)
	if attr == nil {
		var items []hclwrite.Tokens
		for _, d := range append(def, sel) {
			items = append(items, hclwrite.TokensForTraversal(d))
		}
		setAttr(body, name, hclwrite.TokensForTuple(items))
		return nil
	}
	l, ok := splitList(attr.Expr().BuildTokens(nil))
	if !ok {
		return fmt.Errorf("%s is not a list literal; edit it by hand", name)
	}
	l.add(hclwrite.TokensForTraversal(sel))
	body.SetAttributeRaw(name, l.tokens())
	alignAttr(body, name)
	return nil
}

// removeBlocks returns f without the blocks rm, each with the blank line that separated it from
// what follows, or, at the end of a body, from what precedes it.
func removeBlocks(f *hclwrite.File, rm []*hclwrite.Block) (*hclwrite.File, error) {
	drop := map[*hclwrite.Token]bool{}
	for _, b := range rm {
		for _, t := range b.BuildTokens(nil) {
			drop[t] = true
		}
	}
	blank := func(out hclwrite.Tokens) bool {
		return len(out) == 0 || bytes.HasSuffix(out[max(0, len(out)-2):].Bytes(), []byte("\n\n"))
	}
	var out hclwrite.Tokens
	removed := false
	for _, t := range f.BuildTokens(nil) {
		switch {
		case drop[t]:
			removed = true
			continue
		case removed && t.Type == hclsyntax.TokenNewline && blank(out):
			continue
		case removed && t.Type == hclsyntax.TokenCBrace && len(out) > 0 && out[len(out)-1].Type == hclsyntax.TokenNewline && blank(out):
			out = out[:len(out)-1]
		}
		removed = false
		out = append(out, t)
	}
	for removed && len(out) > 0 && out[len(out)-1].Type == hclsyntax.TokenNewline && (len(out) == 1 || blank(out)) {
		out = out[:len(out)-1]
	}
	return parse("", out.Bytes())
}

// expand rewrites block b of f, if it is on one line, onto several lines so it can take more
// attributes and blocks. It returns the file and the block, which are new when b was expanded.
func expand(f *hclwrite.File, b *hclwrite.Block) (*hclwrite.File, *hclwrite.Block, error) {
	toks := b.BuildTokens(nil)
	open := slices.IndexFunc(toks, func(t *hclwrite.Token) bool { return t.Type == hclsyntax.TokenOBrace })
	end := len(toks) - 1
	for end > 0 && toks[end].Type != hclsyntax.TokenCBrace {
		end--
	}
	if open < 0 || slices.ContainsFunc(toks[open:end], func(t *hclwrite.Token) bool { return isSpace(t) }) {
		return f, b, nil
	}
	indent := toks[0].SpacesBefore
	var out hclwrite.Tokens
	for _, t := range f.BuildTokens(nil) {
		switch t {
		case toks[open]:
			out = append(out, t, tok(hclsyntax.TokenNewline, "\n"))
			if open+1 < end {
				toks[open+1].SpacesBefore = indent + 2
			}
			continue
		case toks[end]:
			if open+1 < end {
				out = append(out, tok(hclsyntax.TokenNewline, "\n"))
			}
			t.SpacesBefore = indent
		}
		out = append(out, t)
	}
	nf, err := parse("", out.Bytes())
	if err != nil {
		return nil, nil, err
	}
	nb := findBlock(nf.Body(), b.Type(), labelsOf(b))
	if nb == nil {
		return nil, nil, fmt.Errorf("expanding block %s %v lost it", b.Type(), labelsOf(b))
	}
	return nf, nb, nil
}

// attrsBeforeBlocks moves each attribute of names in top-level block typ labels of f that
// follows a nested block to before the first nested block, after the attributes there, as
// canonical format orders them; hclwrite appends a new attribute at the end of a body.
func attrsBeforeBlocks(f *hclwrite.File, typ string, labels, names []string) (*hclwrite.File, error) {
	b := findBlock(f.Body(), typ, labels)
	nested := b.Body().Blocks()
	if len(nested) == 0 {
		return f, nil
	}
	toks := f.BuildTokens(nil)
	firstBlock := slices.Index(toks, nested[0].BuildTokens(nil)[0])
	start := slices.Index(toks, b.BuildTokens(nil)[0])
	// The block is on several lines, so its { is followed by a newline.
	insert := start + slices.IndexFunc(toks[start:], func(t *hclwrite.Token) bool { return t.Type == hclsyntax.TokenOBrace }) + 2
	var movedNames []string
	for _, name := range sortedAttrs(b.Body()) {
		at := b.Body().GetAttribute(name).BuildTokens(nil)
		i := slices.Index(toks, at[0])
		switch {
		case i < firstBlock:
			insert = max(insert, slices.Index(toks, at[len(at)-1])+1)
		case slices.Contains(names, name):
			movedNames = append(movedNames, name)
		}
	}
	if len(movedNames) == 0 {
		return f, nil
	}
	moved := map[*hclwrite.Token]bool{}
	for _, name := range movedNames {
		for _, t := range b.Body().GetAttribute(name).BuildTokens(nil) {
			moved[t] = true
		}
	}
	var move hclwrite.Tokens
	for _, t := range toks {
		if moved[t] {
			move = append(move, t)
		}
	}
	var out hclwrite.Tokens
	for i, t := range toks {
		if i == insert {
			out = append(out, move...)
		}
		if !moved[t] {
			out = append(out, t)
		}
	}
	nf, err := parse("", out.Bytes())
	if err != nil {
		return nil, err
	}
	body := findBlock(nf.Body(), typ, labels).Body()
	for _, name := range movedNames {
		alignAttr(body, name)
	}
	return nf, nil
}

// findBlock returns the first block of body with type typ and labels.
func findBlock(body *hclwrite.Body, typ string, labels []string) *hclwrite.Block {
	for _, b := range body.Blocks() {
		if b.Type() == typ && slices.Equal(labelsOf(b), labels) {
			return b
		}
	}
	return nil
}

// labelsOf returns the labels of b. hclwrite's Block.Labels drops a quoted label whose literal
// the lexer splits, as it does at % and $, so a label is read from its tokens here.
func labelsOf(b *hclwrite.Block) []string {
	var out []string
	var lit []byte
	toks := b.BuildTokens(nil)
	typ := slices.IndexFunc(toks, func(t *hclwrite.Token) bool { return t.Type == hclsyntax.TokenIdent })
	for _, t := range toks[typ+1:] {
		switch t.Type {
		case hclsyntax.TokenOBrace:
			return out
		case hclsyntax.TokenIdent:
			out = append(out, string(t.Bytes))
		case hclsyntax.TokenOQuote:
			lit = []byte{}
		case hclsyntax.TokenQuotedLit:
			lit = append(lit, t.Bytes...)
		case hclsyntax.TokenCQuote:
			s, diags := hclsyntax.ParseStringLiteralToken(hclsyntax.Token{Type: hclsyntax.TokenQuotedLit, Bytes: lit})
			if diags.HasErrors() {
				s = "\x00invalid"
			}
			out = append(out, s)
		}
	}
	return out
}
