package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/hashicorp/hcl/v2"
	"go.yaml.in/yaml/v3"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/spec"
	"github.com/nvcnvn/dg/internal/view"
)

// LockFile is the name of the lock file (spec 4.7).
const LockFile = "dg.lock"

// Lock is a lock file: the installed packs by local name.
type Lock struct {
	Packs map[string]LockEntry
}

// LockEntry is what dg pack installed under one local name.
type LockEntry struct {
	Source  string `yaml:"source"`
	Version string `yaml:"version"`
	Commit  string `yaml:"commit"`
	Hash    string `yaml:"hash"`
}

var (
	commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hashRE   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// IsCommit reports whether s is a full commit hash, which dg takes to be SHA-1 (spec 4.7).
func IsCommit(s string) bool {
	return commitRE.MatchString(s)
}

// DecodeLock decodes a lock file; name names it in diagnostics. An entry with a problem is left
// out of the result.
func DecodeLock(src []byte, name string) (*Lock, hcl.Diagnostics) {
	d := view.NewYAMLDecoder(src, name, "Invalid lock file")
	lock := &Lock{Packs: map[string]LockEntry{}}
	root := d.Document()
	if root == nil {
		return lock, d.Diags
	}
	version := false
	d.Fields(root, []string{"version", "packs"}, func(key string, _, v *yaml.Node) {
		if key == "version" {
			version = true
			if n := d.Int(v); n != nil && *n != 1 {
				d.Errorf(v, "Version %d is not supported; the version is 1. %s", *n, diag.Newer())
			}
			return
		}
		if !d.Mapping(v) {
			return
		}
		for i := 0; i+1 < len(v.Content); i += 2 {
			k := v.Content[i]
			if problem := LocalNameProblem(k.Value); problem != "" {
				d.Errorf(k, "The pack name %q %s.", k.Value, problem)
				continue
			}
			if slices.ContainsFunc(slices.Collect(maps.Keys(lock.Packs)), func(name string) bool { return strings.EqualFold(name, k.Value) }) {
				d.Errorf(k, "Pack %q is repeated, or differs from another only by case, and so shares its directory.", k.Value)
				continue
			}
			if e, ok := lockEntry(d, k, v.Content[i+1]); ok {
				lock.Packs[k.Value] = e
			}
		}
	})
	if !version {
		d.Errorf(root, "Missing version: 1.")
	}
	return lock, d.Diags
}

func lockEntry(d *view.YAMLDecoder, key, v *yaml.Node) (LockEntry, bool) {
	var e LockEntry
	if !d.Mapping(v) {
		return e, false
	}
	before := len(d.Diags)
	fields := map[string]*string{"source": &e.Source, "version": &e.Version, "commit": &e.Commit, "hash": &e.Hash}
	d.Fields(v, slices.Sorted(maps.Keys(fields)), func(field string, _, v *yaml.Node) {
		if d.Decode(v, fields[field]) {
			switch {
			case field == "commit" && !commitRE.MatchString(e.Commit):
				d.Errorf(v, "The commit %q is not a full commit hash.", e.Commit)
			case field == "hash" && !hashRE.MatchString(e.Hash):
				d.Errorf(v, "The hash %q is not sha256: and 64 hexadecimal digits.", e.Hash)
			}
		}
	})
	for _, field := range slices.Sorted(maps.Keys(fields)) {
		if *fields[field] == "" {
			d.Errorf(key, "Pack %q has no %s.", key.Value, field)
		}
	}
	return e, len(d.Diags) == before
}

// Encode returns l in canonical form.
func (l *Lock) Encode() []byte {
	packs := &yaml.Node{Kind: yaml.MappingNode}
	for _, name := range slices.Sorted(maps.Keys(l.Packs)) {
		var entry yaml.Node
		if err := entry.Encode(l.Packs[name]); err != nil {
			panic(fmt.Sprintf("encoding a lock entry: %v", err))
		}
		packs.Content = append(packs.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, &entry)
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"}, {Kind: yaml.ScalarNode, Value: "1"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "packs"}, packs,
	}}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		panic(fmt.Sprintf("encoding a lock file: %v", err))
	}
	return buf.Bytes()
}

// LocalNameProblem says why name cannot be an installed pack's local name, or returns "".
func LocalNameProblem(name string) string {
	switch {
	case !spec.IsSimpleID(name):
		return "is not a simple ID (letters, digits, _ and -, starting with a letter or _)"
	case name == "generic":
		return "is the built-in pack's"
	}
	return ""
}

// ReadPack returns the files of the pack in directory dir of fsys, by their paths in the pack: the
// set spec 4.7 says a pack holds, which is all that is hashed, copied or read. It fails on a pack
// with neither spec.hcl nor style.hcl, or with one of those files a link or not a regular file.
func ReadPack(fsys fs.FS, dir string) (fstest.MapFS, error) {
	pack := fstest.MapFS{}
	err := fs.WalkDir(fsys, dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel := strings.TrimPrefix(name, dir+"/")
		if !InPack(rel) {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is a symbolic link or not a regular file", rel)
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		pack[rel] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if pack["spec.hcl"] == nil && pack["style.hcl"] == nil {
		return nil, errors.New("it holds neither spec.hcl nor style.hcl")
	}
	return pack, nil
}

// InPack reports whether the path rel, relative to a pack's root, is in the set spec 4.7 says a
// pack holds.
func InPack(rel string) bool {
	if strings.HasSuffix(rel, ".svg") {
		return true
	}
	return !strings.Contains(rel, "/") && (rel == "spec.hcl" || rel == "style.hcl" ||
		strings.HasPrefix(rel, "README") || strings.HasPrefix(rel, "LICENSE") || strings.HasPrefix(rel, "NOTICE"))
}

// PackHash returns the hash of a pack as ReadPack gives it, as dg.lock holds it: the SHA-256 of
// its sorted paths and contents, with CRLF read as LF.
func PackHash(pack fstest.MapFS) string {
	h := sha256.New()
	for _, name := range slices.Sorted(maps.Keys(pack)) {
		data := bytes.ReplaceAll(pack[name].Data, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(h, "%q %d\n", name, len(data))
		h.Write(data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
