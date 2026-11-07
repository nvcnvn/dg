package ops

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/project"
)

// Pack is what dg pack fetched: a pack's files and its entry in dg.lock, under its local name.
type Pack struct {
	Name  string
	Entry project.LockEntry
	Files fstest.MapFS
}

// packEdit is a project while dg pack changes it, which may have any error but one that hides
// read_only or what dg.lock holds: dg pack is how a project with a broken pack is repaired.
type packEdit struct {
	base      fs.FS
	p         *project.Project
	config    *hclwrite.File // nil without dg.hcl
	edited    bool           // the edit changed config; a read and rewrite alone may not be exact
	noConfig  bool           // the edit removes dg.hcl
	lock      *project.Lock
	lockFile  []byte
	lockEdits bool
	dirs      map[string]map[string][]byte
}

func loadPacks(base fs.FS) (*packEdit, error) {
	p, diags, err := project.LoadFS(base)
	if err != nil {
		return nil, err
	}
	if p.ReadOnlyUnknown {
		var errs []string
		for _, d := range diags {
			if d.File == project.ProjectFile && d.Severity == diag.Error {
				errs = append(errs, d.String())
			}
		}
		return nil, fmt.Errorf("dg.hcl has errors, so read_only may be incomplete; fix them first:\n  %s", strings.Join(errs, "\n  "))
	}
	e := &packEdit{base: base, p: p, dirs: map[string]map[string][]byte{}, lock: &project.Lock{Packs: map[string]project.LockEntry{}}}
	config, err := fs.ReadFile(base, project.ProjectFile)
	switch {
	case err == nil:
		if e.config, err = parse(project.ProjectFile, config); err != nil {
			return nil, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	e.lockFile, err = fs.ReadFile(base, project.LockFile)
	switch {
	case err == nil:
		lock, d := project.DecodeLock(e.lockFile, project.LockFile)
		if d.HasErrors() {
			return nil, fmt.Errorf("%s has errors, so what it holds would be lost; fix it or restore it from git: %s", project.LockFile, d.Error())
		}
		e.lock = lock
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	return e, nil
}

func packDir(name string) string {
	return path.Join("packs", name)
}

func (e *packEdit) projectBlock() *hclwrite.Block {
	if e.config == nil {
		return nil
	}
	if found := blocks(e.config.Body(), "project"); len(found) > 0 {
		return found[0]
	}
	return nil
}

func (e *packEdit) packBlocks() []*hclwrite.Block {
	if b := e.projectBlock(); b != nil {
		return blocks(b.Body(), "pack")
	}
	return nil
}

func (e *packEdit) packBlock(name string) *hclwrite.Block {
	for _, b := range e.packBlocks() {
		if slices.Equal(labelsOf(b), []string{name}) {
			return b
		}
	}
	return nil
}

// checkLocalName refuses a name that is not a pack's.
func checkLocalName(name string) error {
	if problem := project.LocalNameProblem(name); problem != "" {
		return fmt.Errorf("the pack name %q %s", name, problem)
	}
	return nil
}

// checkFree refuses a new pack's name that is taken, on a disk that ignores case as well as on
// one that does not: packs/AWS is packs/aws there.
func (e *packEdit) checkFree(name string) error {
	if err := checkLocalName(name); err != nil {
		return err
	}
	taken := func(other string) error {
		if other == name {
			return fmt.Errorf("the pack %q is already installed; dg pack update replaces it", name)
		}
		return fmt.Errorf("the pack %q differs only by case from %q, and they would share a directory; choose another name with --as", name, other)
	}
	for _, b := range e.packBlocks() {
		if labels := labelsOf(b); len(labels) == 1 && strings.EqualFold(labels[0], name) {
			return taken(labels[0])
		}
	}
	for _, other := range slices.Sorted(maps.Keys(e.lock.Packs)) {
		if strings.EqualFold(other, name) {
			return taken(other)
		}
	}
	entries, err := fs.ReadDir(e.base, "packs")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), name) {
			return fmt.Errorf("%s already exists and belongs to no installed pack, and an installed pack's directory must not hold a local pack; choose another name with --as", packDir(entry.Name()))
		}
	}
	return nil
}

// checkNotLocal refuses to replace or remove a directory that packs in dg.hcl lists.
func (e *packEdit) checkNotLocal(name string) error {
	for _, dir := range e.p.PackDirs {
		if strings.EqualFold(dir, packDir(name)) && !slices.ContainsFunc(e.p.Installed, func(in *project.Installed) bool { return in.Dir == dir }) {
			return fmt.Errorf("%s is a local pack, which packs in dg.hcl lists", dir)
		}
	}
	return nil
}

func (e *packEdit) installed(name string) (*project.Installed, error) {
	if err := checkLocalName(name); err != nil {
		return nil, err
	}
	for _, in := range e.p.Installed {
		if in.Name == name {
			return in, nil
		}
	}
	return nil, fmt.Errorf("dg.hcl has no pack %q", name)
}

// CheckAdd refuses what AddPack would refuse, for a name not yet fetched.
func CheckAdd(base fs.FS, name string) error {
	e, err := loadPacks(base)
	if err != nil {
		return err
	}
	return e.checkFree(name)
}

// CheckUpdate refuses what UpdatePack would refuse, and returns the source of the pack's block.
func CheckUpdate(base fs.FS, name string) (string, error) {
	e, err := loadPacks(base)
	if err != nil {
		return "", err
	}
	in, err := e.installed(name)
	if err != nil {
		return "", err
	}
	if in.Source == "" {
		return "", fmt.Errorf("the pack %q has no source in dg.hcl", name)
	}
	return in.Source, nil
}

// LockedPacks returns what dg.lock holds, refusing what InstallPacks would refuse.
func LockedPacks(base fs.FS) (map[string]project.LockEntry, error) {
	e, err := loadPacks(base)
	if err != nil {
		return nil, err
	}
	for _, name := range slices.Sorted(maps.Keys(e.lock.Packs)) {
		if err := e.checkNotLocal(name); err != nil {
			return nil, err
		}
	}
	return e.lock.Packs, nil
}

// addBlock adds pack's block to the project block, creating that if there is none. Every new
// separator is a single newline, so that removeBlock takes out exactly what this put in; a block
// this creates has an empty label, which is how removeBlock knows it.
func (e *packEdit) addBlock(pack Pack) error {
	e.edited = true
	b := hclwrite.NewBlock("pack", []string{pack.Name})
	b.Body().SetAttributeValue("source", cty.StringVal(pack.Entry.Source))
	b.Body().SetAttributeValue("version", cty.StringVal(pack.Entry.Version))
	target := e.projectBlock()
	if target == nil {
		created := hclwrite.NewBlock("project", []string{""})
		created.Body().AppendBlock(b)
		if e.config == nil {
			e.config = hclwrite.NewEmptyFile()
		} else {
			e.config.Body().AppendNewline()
		}
		e.config.Body().AppendBlock(newBlock(created, 0))
		return nil
	}
	expanded, multi, err := expand(e.config, target)
	if err != nil {
		return err
	}
	if multi == target || len(target.Body().BuildTokens(nil)) > 0 {
		multi.Body().AppendNewline()
	}
	e.config = expanded
	appendNested(multi.Body(), b)
	return nil
}

// removeBlock undoes addBlock for the block of pack name. A pack block that came first in a body
// has no separator before it: what follows it gives up the one it has, and a body left empty
// closes up to {}, as expand found it. The project block goes too when addBlock created it.
func (e *packEdit) removeBlock(name string) error {
	b := e.packBlock(name)
	if b == nil {
		return nil
	}
	e.edited = true
	toks := e.config.BuildTokens(nil)
	own := b.BuildTokens(nil)
	at := slices.Index(toks, own[0])
	before, after := slices.Clone(toks[:at]), toks[at+len(own):]
	empty := false
	switch {
	case at >= 2 && toks[at-2].Type == hclsyntax.TokenOBrace && after[0].Type == hclsyntax.TokenCBrace:
		before, empty = before[:at-1], true
		after[0].SpacesBefore = 0
	case at >= 2 && toks[at-2].Type == hclsyntax.TokenOBrace && after[0].Type == hclsyntax.TokenNewline:
		after = after[1:]
	case at >= 2 && toks[at-1].Type == hclsyntax.TokenNewline && isLineEnd(toks[at-2]):
		before = before[:at-1]
	}
	config, err := parse(project.ProjectFile, append(before, after...).Bytes())
	if err != nil {
		return err
	}
	e.config = config
	if pb := e.projectBlock(); empty && slices.Equal(labelsOf(pb), []string{""}) {
		return e.removeProject(pb)
	}
	return nil
}

// removeProject takes block b, which addBlock created, out of dg.hcl, with the newline before it
// or, if nothing else is left and nothing was there before it, with the file.
func (e *packEdit) removeProject(b *hclwrite.Block) error {
	toks := e.config.BuildTokens(nil)
	own := b.BuildTokens(nil)
	// A comment on the line above is the block's to hclwrite, and is not addBlock's.
	first := slices.IndexFunc(own, func(t *hclwrite.Token) bool { return t.Type == hclsyntax.TokenIdent })
	at := slices.Index(toks, own[first])
	before, after := slices.Clone(toks[:at]), toks[at+len(own)-first:]
	switch last := len(before) - 1; {
	case at == 0:
		if len(hclwrite.Tokens(after).Bytes()) == 0 {
			e.noConfig = true
			return nil
		}
	case before[last].Type == hclsyntax.TokenNewline:
		before = before[:last]
	case before[last].Type == hclsyntax.TokenComment:
		before[last].Bytes = bytes.TrimSuffix(before[last].Bytes, []byte("\n"))
	}
	config, err := parse(project.ProjectFile, append(before, after...).Bytes())
	if err != nil {
		return err
	}
	e.config = config
	return nil
}

func (e *packEdit) putPack(pack Pack) {
	content := map[string][]byte{}
	for name, f := range pack.Files {
		content[name] = f.Data
	}
	e.dirs[packDir(pack.Name)] = content
}

func (e *packEdit) setEntry(pack Pack) {
	e.lock.Packs[pack.Name] = pack.Entry
	e.lockEdits = true
}

// writePacks applies edit to the project in d and writes the result, all together, as the only
// writer of the project. It alone of the ways to write a project writes an installed pack's
// files, which read_only can still forbid.
func writePacks(d fsys.Disk, edit func(*packEdit) error) error {
	unlock, err := d.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	e, err := loadPacks(d)
	if err != nil {
		return err
	}
	if err := edit(e); err != nil {
		return err
	}
	out := map[string][]byte{}
	switch {
	case e.noConfig:
		out[project.ProjectFile] = nil
	case e.edited:
		out[project.ProjectFile] = append([]byte{}, source(e.config)...)
	}
	if e.lockEdits {
		var after []byte
		if len(e.lock.Packs) > 0 {
			after = e.lock.Encode()
		}
		if !bytes.Equal(after, e.lockFile) {
			out[project.LockFile] = after
		}
	}
	touched := slices.Collect(maps.Keys(out))
	for dir, content := range e.dirs {
		for name := range content {
			touched = append(touched, path.Join(dir, name))
		}
		// A directory that cannot be walked holds nothing a pattern could name in the project.
		_ = fs.WalkDir(d, dir, func(name string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				touched = append(touched, name)
			}
			return nil
		})
	}
	slices.Sort(touched)
	for _, name := range touched {
		if e.p.MatchesReadOnly(name) {
			return fmt.Errorf("%s is read-only (read_only in dg.hcl); dg pack would change it", name)
		}
	}
	return d.WriteDirs(out, e.dirs)
}

// AddPack installs a new pack: its block in dg.hcl, its entry in dg.lock and its files.
func AddPack(d fsys.Disk, pack Pack) error {
	return writePacks(d, func(e *packEdit) error {
		if err := e.checkFree(pack.Name); err != nil {
			return err
		}
		if err := e.addBlock(pack); err != nil {
			return err
		}
		e.setEntry(pack)
		e.putPack(pack)
		return nil
	})
}

// UpdatePack replaces the files of the installed pack, and sets its block's version and its
// entry in dg.lock.
func UpdatePack(d fsys.Disk, pack Pack) error {
	return writePacks(d, func(e *packEdit) error {
		in, err := e.installed(pack.Name)
		if err != nil {
			return err
		}
		if in.Version != pack.Entry.Version {
			setValue(e.packBlock(pack.Name).Body(), "version", cty.StringVal(pack.Entry.Version))
			e.edited = true
		}
		e.setEntry(pack)
		e.putPack(pack)
		return nil
	})
}

// RemovePack removes a pack's directory, block and dg.lock entry, and dg.lock if it was the last:
// dg never writes a lock with no packs, so one that held only this pack did not exist before it.
func RemovePack(d fsys.Disk, name string) error {
	return writePacks(d, func(e *packEdit) error {
		if err := checkLocalName(name); err != nil {
			return err
		}
		_, locked := e.lock.Packs[name]
		if e.packBlock(name) == nil && !locked {
			return fmt.Errorf("dg.hcl and %s have no pack %q", project.LockFile, name)
		}
		if err := e.checkNotLocal(name); err != nil {
			return err
		}
		delete(e.lock.Packs, name)
		e.lockEdits = true
		e.dirs[packDir(name)] = nil
		return e.removeBlock(name)
	})
}

// InstallPacks replaces the files of packs that dg.lock holds, and changes neither dg.hcl nor
// dg.lock.
func InstallPacks(d fsys.Disk, packs []Pack) error {
	return writePacks(d, func(e *packEdit) error {
		for _, pack := range packs {
			if err := e.checkNotLocal(pack.Name); err != nil {
				return err
			}
			e.putPack(pack)
		}
		return nil
	})
}
