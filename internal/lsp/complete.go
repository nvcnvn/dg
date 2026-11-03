package lsp

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/nvcnvn/dg/internal/model"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/style"
)

// frame is a block or an object constructor open at the cursor.
type frame struct {
	block bool
	typ   string
	label string
	stmt  int // index of the token starting the statement the cursor is in
	depth int // brackets and parentheses open in that statement
}

// cursor is where the cursor is: the types of the blocks around it, innermost last, and the
// tokens of the statement it is in, up to it.
type cursor struct {
	blocks []string
	label  string // of the innermost block
	stmt   hclsyntax.Tokens
}

// cursorContext lexes text up to off and finds the blocks and statement the cursor is in. It
// works on text that does not parse, as text being typed rarely does.
func cursorContext(text string, off int) cursor {
	toks := slices.DeleteFunc(lex([]byte(text[:off])), func(t hclsyntax.Token) bool {
		return t.Type == hclsyntax.TokenComment || t.Type == hclsyntax.TokenEOF
	})
	stack := []frame{{block: true}}
	for i, t := range toks {
		top := &stack[len(stack)-1]
		switch t.Type {
		case hclsyntax.TokenOBrace:
			if typ, label, ok := blockHeader(toks[top.stmt:i]); ok && top.block && top.depth == 0 {
				stack = append(stack, frame{block: true, typ: typ, label: label, stmt: i + 1})
			} else {
				stack = append(stack, frame{stmt: top.stmt})
			}
		case hclsyntax.TokenCBrace:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
				if top := &stack[len(stack)-1]; top.block && top.depth == 0 {
					top.stmt = i + 1
				}
			}
		case hclsyntax.TokenOBrack, hclsyntax.TokenOParen:
			top.depth++
		case hclsyntax.TokenCBrack, hclsyntax.TokenCParen:
			top.depth = max(top.depth-1, 0)
		case hclsyntax.TokenNewline:
			if top.block && top.depth == 0 {
				top.stmt = i + 1
			}
		}
	}
	var c cursor
	stmt := 0
	for _, f := range stack {
		if f.block {
			if f.typ != "" {
				c.blocks = append(c.blocks, f.typ)
				c.label = f.label
			}
			stmt = f.stmt
		}
	}
	c.stmt = toks[min(stmt, len(toks)):]
	return c
}

// lex lexes src, ending a quoted string at the end of its line as a string cannot span lines,
// so a string left open while typing does not swallow the rest of the text.
func lex(src []byte) hclsyntax.Tokens {
	var out hclsyntax.Tokens
	for {
		toks, _ := hclsyntax.LexConfig(src, "", hcl.InitialPos)
		i := slices.IndexFunc(toks, func(t hclsyntax.Token) bool { return t.Type == hclsyntax.TokenQuotedNewline })
		if i < 0 {
			return append(out, toks...)
		}
		out = append(out, toks[:i]...)
		out = append(out, hclsyntax.Token{Type: hclsyntax.TokenCQuote}, hclsyntax.Token{Type: hclsyntax.TokenNewline})
		src = src[toks[i].Range.End.Byte:]
	}
}

// blockHeader reports whether toks are a block's type and labels, returning the type and the
// first label.
func blockHeader(toks hclsyntax.Tokens) (typ, label string, ok bool) {
	if len(toks) == 0 || toks[0].Type != hclsyntax.TokenIdent {
		return "", "", false
	}
	for i, t := range toks[1:] {
		switch t.Type {
		case hclsyntax.TokenIdent, hclsyntax.TokenQuotedLit:
			if label == "" && (i == 0 || toks[i].Type == hclsyntax.TokenOQuote) {
				label = string(t.Bytes)
			}
		case hclsyntax.TokenOQuote, hclsyntax.TokenCQuote:
		default:
			return "", "", false
		}
	}
	return string(toks[0].Bytes), label, true
}

func (c cursor) in(typ string) bool {
	return slices.Contains(c.blocks, typ)
}

func (c cursor) innermost() string {
	if len(c.blocks) == 0 {
		return ""
	}
	return c.blocks[len(c.blocks)-1]
}

// openString returns the text typed so far in a string the statement ends in, if it ends in
// one that is not closed.
func openString(toks hclsyntax.Tokens) (string, bool) {
	n := len(toks)
	switch {
	case n >= 1 && toks[n-1].Type == hclsyntax.TokenOQuote:
		return "", true
	case n >= 2 && toks[n-1].Type == hclsyntax.TokenQuotedLit && toks[n-2].Type == hclsyntax.TokenOQuote:
		return string(toks[n-1].Bytes), true
	}
	return "", false
}

// traversal parses the traversal the statement ends in: its complete steps and the name being
// typed after them, or, with no steps, the start of a value. It reports false unless the
// statement ends in one of those.
func traversal(toks hclsyntax.Tokens) (steps []string, partial string, ok bool) {
	i := len(toks)
	is := func(j int, types ...hclsyntax.TokenType) bool {
		return j >= 0 && j < len(toks) && slices.Contains(types, toks[j].Type)
	}
	if is(i-1, hclsyntax.TokenIdent) {
		partial = string(toks[i-1].Bytes)
		i--
	}
	if !is(i-1, hclsyntax.TokenDot) {
		return nil, partial, i == 0 || is(i-1, hclsyntax.TokenEqual, hclsyntax.TokenOBrack, hclsyntax.TokenComma)
	}
	i--
	for {
		if is(i-5, hclsyntax.TokenOBrack) && is(i-4, hclsyntax.TokenOQuote) && is(i-3, hclsyntax.TokenQuotedLit) &&
			is(i-2, hclsyntax.TokenCQuote) && is(i-1, hclsyntax.TokenCBrack) {
			name, err := strconv.Unquote(`"` + string(toks[i-3].Bytes) + `"`)
			if err != nil {
				return nil, "", false
			}
			steps = append([]string{name}, steps...)
			i -= 5
			continue
		}
		if !is(i-1, hclsyntax.TokenIdent) {
			return nil, "", false
		}
		steps = append([]string{string(toks[i-1].Bytes)}, steps...)
		i--
		if !is(i-1, hclsyntax.TokenDot) {
			return steps, partial, true
		}
		i--
	}
}

// wordEnd returns the offset where the word at off ends: the rest of an identifier, or of a
// string up to its closing quote.
func wordEnd(text string, off int, inString bool) int {
	for off < len(text) {
		c := text[off]
		if inString && (c == '"' || c == '\n' || c == '\\') ||
			!inString && !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80) {
			break
		}
		off++
	}
	return off
}

// completer builds the completion items for one request.
type completer struct {
	p     *project.Project
	lines *lines
	text  string
	start int // where the word being completed starts
	end   int // where it ends, past the cursor
	items []completionItem
}

func (c *completer) add(label string, kind int, detail string) *completionItem {
	rng := c.lines.rng(c.start, c.end)
	c.items = append(c.items, completionItem{Label: label, Kind: kind, Detail: detail, TextEdit: &TextEdit{Range: rng, NewText: label}})
	return &c.items[len(c.items)-1]
}

func (s *Server) completion(l *loaded, rel string, pos Position) (any, error) {
	ls := l.linesOf(rel)
	off := ls.offset(pos)
	ctx := cursorContext(ls.text, off)
	c := &completer{p: s.parsed(l), lines: ls, text: ls.text, start: off, end: off}
	role := ""
	for _, f := range l.p.Files {
		if f.Path == rel {
			role = f.Role
		}
	}
	c.complete(ctx, role)
	slices.SortStableFunc(c.items, func(a, b completionItem) int { return strings.Compare(a.Label, b.Label) })
	if c.items == nil {
		return []completionItem{}, nil
	}
	return c.items, nil
}

var styleBlocks = map[string]map[string]style.Prop{"style": style.NodeProps, "edge_style": style.EdgeProps, "overlay_style": style.OverlayProps}

func (c *completer) complete(ctx cursor, role string) {
	st := ctx.stmt
	props := styleBlocks[ctx.innermost()]
	if role == project.RoleViews && !ctx.in("view") {
		props = nil
	}
	if s, ok := openString(st); ok {
		c.start -= len(s)
		c.end = wordEnd(c.text, c.end, true)
		switch {
		case len(st) <= 3 && st[0].Type == hclsyntax.TokenIdent && st[1].Type == hclsyntax.TokenOQuote:
			c.styleLabels(string(st[0].Bytes), ctx, role)
		case len(st) >= 3 && st[1].Type == hclsyntax.TokenEqual:
			c.stringValue(string(st[0].Bytes), ctx, role, props)
		}
		return
	}
	steps, partial, ok := traversal(st)
	if !ok {
		return
	}
	c.start -= len(partial)
	c.end = wordEnd(c.text, c.end, false)
	switch {
	case len(st) <= 1 && steps == nil:
		for _, name := range slices.Sorted(maps.Keys(props)) {
			c.add(name, itemProperty, "")
		}
	case len(st) < 2 || st[0].Type != hclsyntax.TokenIdent || st[1].Type != hclsyntax.TokenEqual:
	case steps == nil:
		c.root(string(st[0].Bytes), ctx, role, props)
	default:
		exclude := ""
		if string(st[0].Bytes) == "parent" && ctx.innermost() == "node" {
			exclude = ctx.label
		}
		c.steps(steps, exclude)
	}
}

// styleLabels completes the label of a style or edge_style block.
func (c *completer) styleLabels(block string, ctx cursor, role string) {
	inView := role == project.RoleViews && ctx.innermost() == "view"
	if role != project.RoleStyle && !inView || len(ctx.blocks) > 1 || role == project.RoleStyle && len(ctx.blocks) > 0 {
		return
	}
	voc := c.p.Vocabulary()
	switch block {
	case "style":
		for _, k := range voc.Kinds {
			c.add(k.Name, itemClass, "kind")
		}
		if inView {
			for id, n := range c.p.Model.Nodes {
				c.add("node."+id, itemVariable, n.Kind)
			}
		}
	case "edge_style":
		for _, k := range voc.EdgeKinds {
			c.add(k, itemInterface, "edge kind")
		}
		if inView {
			for id, e := range c.p.Model.Edges {
				c.add("edge."+id, itemVariable, e.Kind)
			}
		}
	default:
		return
	}
	for _, t := range voc.Tags {
		c.add("tag."+t, itemEnumMember, "tag")
	}
}

// stringValue completes the string value of attribute attr.
func (c *completer) stringValue(attr string, ctx cursor, role string, props map[string]style.Prop) {
	block := ctx.innermost()
	switch {
	case role == project.RoleModel && attr == "kind" && block == "node", role == project.RoleSpec && attr == "extends" && block == "kind":
		for name := range c.p.Spec.Kinds {
			c.add(name, itemClass, "kind")
		}
	case role == project.RoleModel && attr == "kind" && block == "edge":
		for name := range c.p.Spec.EdgeKinds {
			c.add(name, itemInterface, "edge kind")
		}
	case props != nil:
		for _, v := range c.values(attr, props) {
			if s, err := strconv.Unquote(v); err == nil && strings.HasPrefix(v, `"`) {
				c.add(s, itemEnumMember, "")
			}
		}
	}
}

// values returns the values property attr takes, as HCL expressions.
func (c *completer) values(attr string, props map[string]style.Prop) []string {
	values := props[attr].Values
	if attr == "shape" {
		values = slices.Clone(values)
		for name := range c.p.Styles.Shapes {
			values = append(values, strconv.Quote(name))
		}
	}
	return values
}

var (
	nodeForms = []string{"node", "kind", "tag", "children", "descendants"}
	edgeForms = []string{"edge_kind", "tag", "edge"}
)

// root completes the start of the value of attribute attr: a property value, or the first step
// of a reference or selector.
func (c *completer) root(attr string, ctx cursor, role string, props map[string]style.Prop) {
	var forms []string
	switch {
	case props != nil:
		for _, v := range c.values(attr, props) {
			c.add(v, itemEnumMember, "")
		}
		return
	case role == project.RoleModel && (attr == "parent" && ctx.innermost() == "node" || (attr == "from" || attr == "to") && ctx.innermost() == "edge"):
		forms = []string{"node"}
	case role == project.RoleViews && attr == "of" && ctx.innermost() == "view":
		forms = []string{"node"}
	case role == project.RoleViews && slices.Contains([]string{"include", "exclude", "collapse"}, attr) && ctx.innermost() == "view",
		role == project.RoleViews && attr == "members" && ctx.innermost() == "overlay":
		c.add("all", itemKeyword, "every node")
		forms = nodeForms
	case role == project.RoleViews && attr == "edges" && ctx.innermost() == "view":
		forms = edgeForms
	}
	for _, f := range forms {
		c.add(f+".", itemKeyword, "").Command = retrigger
	}
}

// steps completes the step after steps of a reference or selector, leaving out node exclude.
func (c *completer) steps(steps []string, exclude string) {
	p := c.p
	switch {
	case len(steps) == 1 && (steps[0] == "node" || steps[0] == "children" || steps[0] == "descendants"):
		for id, n := range p.Model.Nodes {
			if id != exclude && (steps[0] == "node" || p.Spec.IsContainer(n.Kind)) {
				c.nodeID(id, n)
			}
		}
	case len(steps) == 1 && steps[0] == "kind":
		for name := range p.Spec.Kinds {
			c.add(name, itemClass, "kind")
		}
	case len(steps) == 1 && steps[0] == "edge_kind":
		for name := range p.Spec.EdgeKinds {
			c.add(name, itemInterface, "edge kind")
		}
	case len(steps) == 1 && steps[0] == "edge":
		for id, e := range p.Model.Edges {
			c.name(id, e.Kind, itemVariable)
		}
	case len(steps) == 1 && steps[0] == "tag":
		for _, t := range p.Vocabulary().Tags {
			c.add(t, itemEnumMember, "tag")
		}
	case len(steps) == 2 && steps[0] == "node" && p.Model.Nodes[steps[1]] != nil:
		n := p.Model.Nodes[steps[1]]
		if len(c.ports(n)) > 0 {
			c.add("port.", itemKeyword, "").Command = retrigger
		}
		if len(rows(n)) > 0 {
			c.add("row.", itemKeyword, "").Command = retrigger
		}
	case len(steps) == 3 && steps[0] == "node" && p.Model.Nodes[steps[1]] != nil && (steps[2] == "port" || steps[2] == "row"):
		n := p.Model.Nodes[steps[1]]
		names := c.ports(n)
		if steps[2] == "row" {
			names = rows(n)
		}
		for _, name := range names {
			c.name(name, steps[2], itemField)
		}
	}
}

// nodeID adds node id.
func (c *completer) nodeID(id string, n *model.Node) {
	c.name(id, n.Kind, itemVariable).Documentation = "label: " + n.Label
}

// name adds a name after a dot: as it is when it is a simple ID, else in index form, replacing
// the dot.
func (c *completer) name(name, detail string, kind int) *completionItem {
	if spec.IsSimpleID(name) || c.start == 0 || c.text[c.start-1] != '.' {
		return c.add(name, kind, detail)
	}
	start := c.start
	c.start--
	item := c.add(name, kind, detail)
	c.start = start
	item.TextEdit.NewText = "[" + strconv.Quote(name) + "]"
	item.FilterText = "." + name
	return item
}

func (c *completer) ports(n *model.Node) []string {
	names := slices.Clone(c.p.Spec.Ports(n.Kind))
	for _, port := range n.Ports {
		names = append(names, port.Name)
	}
	return names
}

func rows(n *model.Node) []string {
	var names []string
	for _, comp := range n.Compartments {
		for _, r := range comp.Rows {
			names = append(names, r.Name)
		}
	}
	return names
}
