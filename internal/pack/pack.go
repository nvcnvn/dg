// Package pack is dg pack: it fetches installed packs and the index from git repositories
// (spec FR-21), and leaves writing them to internal/ops.
package pack

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing/fstest"
	"unicode"

	"github.com/nvcnvn/dg/internal/diag"
	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/ops"
	"github.com/nvcnvn/dg/internal/project"
	"github.com/nvcnvn/dg/internal/spec"
)

// indexSource is the repository that holds the index (spec 4.7).
const indexSource = "github.com/dg-packs/index"

var (
	hostRE    = regexp.MustCompile(`^[0-9A-Za-z]([0-9A-Za-z.-]*[0-9A-Za-z])?(:[0-9]+)?$`)
	versionRE = regexp.MustCompile(`^[0-9A-Za-z_][0-9A-Za-z_.+@/-]*$`)
)

// location is a parsed source: the repository's URL and the pack's directory in it.
type location struct {
	url, dir string
}

// parseSource parses and refuses a source, which may come from an argument, dg.hcl, dg.lock or
// the index.
func parseSource(s string) (location, error) {
	refuse := func(format string, args ...any) (location, error) {
		return location{}, fmt.Errorf("the source %q %s", s, fmt.Sprintf(format, args...))
	}
	if strings.HasPrefix(s, "-") {
		return refuse("starts with -")
	}
	if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == '\\' || unicode.IsControl(r) }) {
		return refuse("holds a space, a backslash or a control character")
	}
	scheme, rest, isURL := strings.Cut(s, "://")
	if !isURL {
		scheme, rest = "https", s
	}
	repo, dir, inDir := strings.Cut(rest, "//")
	u, err := url.Parse(scheme + "://" + repo)
	if err != nil {
		return refuse("is not a URL")
	}
	_, password := u.User.Password()
	switch {
	case !isURL && !strings.Contains(repo, "/"):
		return refuse("is not <host>/<path>")
	case scheme != "https" && scheme != "ssh" && scheme != "file":
		return refuse("has a scheme other than https, ssh and file")
	case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(repo, "?#"):
		return refuse("has a query or a fragment")
	case password || u.User != nil && scheme != "ssh":
		return refuse("holds credentials, which would be written to dg.hcl; use a credential helper")
	case scheme == "file" && u.Host != "", scheme != "file" && !hostRE.MatchString(u.Host):
		return refuse("has an invalid host")
	case strings.Trim(u.Path, "/") == "":
		return refuse("has no repository path")
	case inDir && (dir == "." || !fs.ValidPath(dir)):
		return refuse("names a directory that is not a path inside the repository")
	}
	return location{url: scheme + "://" + repo, dir: dir}, nil
}

// defaultName is the local name a source gives: its last segment.
func defaultName(s string, loc location) string {
	if loc.dir != "" {
		return path.Base(loc.dir)
	}
	return strings.TrimSuffix(path.Base(strings.TrimRight(s, "/")), ".git")
}

// splitVersion splits a <source>[@<version>] argument. An @ before the path of a URL is its user.
func splitVersion(arg string) (source, version string, ok bool) {
	start := 0
	if _, rest, isURL := strings.Cut(arg, "://"); isURL {
		start = len(arg) - len(rest)
	}
	if slash := strings.IndexByte(arg[start:], '/'); slash >= 0 {
		start += slash
	} else {
		start = 0
	}
	if at := strings.IndexByte(arg[start:], '@'); at >= 0 {
		return arg[:start+at], arg[start+at+1:], true
	}
	return arg, "", false
}

// checkVersion refuses a version that could be anything but a tag or a commit hash to git.
func checkVersion(version string) error {
	if !versionRE.MatchString(version) || strings.Contains(version, "..") {
		return fmt.Errorf("the version %q is not a tag or a full commit hash", version)
	}
	return nil
}

// Installed is what add and update installed.
type Installed struct {
	Name string
	// Licenses are the project paths of the pack's LICENSE* and NOTICE* files.
	Licenses []string
}

func installed(pack ops.Pack) *Installed {
	in := &Installed{Name: pack.Name}
	for _, name := range slices.Sorted(maps.Keys(pack.Files)) {
		if strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "NOTICE") {
			in.Licenses = append(in.Licenses, path.Join("packs", pack.Name, name))
		}
	}
	return in
}

// fetch fetches the pack at source for local name: a tag, by default the highest release, or a
// commit. It returns the pack with the version it took.
func fetch(name, source string, loc location, version string) (ops.Pack, error) {
	return withRepo(func(r *repo) (ops.Pack, error) {
		var commit string
		var err error
		if project.IsCommit(version) {
			commit, err = r.fetchCommit(loc.url, version)
		} else {
			var refs []string
			if refs, err = r.refs(loc.url); err != nil {
				return ops.Pack{}, err
			}
			if version, err = pickTag(refs, version, loc.url); err != nil {
				return ops.Pack{}, err
			}
			commit, err = r.fetchOne(loc.url, "refs/tags/"+version)
		}
		if err != nil {
			return ops.Pack{}, err
		}
		return r.pack(name, source, loc, version, commit)
	})
}

func (r *repo) pack(name, source string, loc location, version, commit string) (ops.Pack, error) {
	tree, err := r.files(commit, loc.dir, project.InPack)
	if err != nil {
		return ops.Pack{}, err
	}
	files, err := project.ReadPack(tree, ".")
	if err != nil {
		return ops.Pack{}, fmt.Errorf("%s at %s is refused: %w", source, version, err)
	}
	return ops.Pack{Name: name, Files: files, Entry: project.LockEntry{Source: source, Version: version, Commit: commit, Hash: project.PackHash(files)}}, nil
}

// Add installs the pack that arg, <source>[@<version>], names in the project in dir, under the
// local name as, by default the source's last segment.
func Add(dir, arg, as string) (*Installed, error) {
	source, version, err := parseAdd(arg)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(source, "/") {
		if source, err = lookup(source); err != nil {
			return nil, err
		}
	}
	loc, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	name := cmp.Or(as, defaultName(source, loc))
	disk := fsys.Disk(dir)
	if err := ops.CheckAdd(disk, name); err != nil {
		return nil, err
	}
	pack, err := fetch(name, source, loc, version)
	if err != nil {
		return nil, err
	}
	if err := ops.AddPack(disk, pack); err != nil {
		return nil, err
	}
	return installed(pack), nil
}

// parseAdd splits arg, refusing a version that is not one and a source with no / that is no
// name for the index; parseSource judges a source with one.
func parseAdd(arg string) (source, version string, err error) {
	source, version, hasVersion := splitVersion(arg)
	if hasVersion {
		if err := checkVersion(version); err != nil {
			return "", "", err
		}
	}
	if !strings.Contains(source, "/") && !spec.IsSimpleID(source) {
		return "", "", fmt.Errorf("the source %q is neither a name in the index nor <host>/<path> nor an https, ssh or file URL", source)
	}
	return source, version, nil
}

func lookup(name string) (string, error) {
	entries, err := fetchIndex()
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(entries, func(e Entry) bool { return e.Name == name })
	if i < 0 {
		return "", fmt.Errorf("the index has no pack %q; dg pack search looks for one, and any git URL works without it", name)
	}
	return entries[i].Source, nil
}

// Update replaces the files of the installed pack name with those of version, by default the
// highest release of its source.
func Update(dir, name, version string) (*Installed, error) {
	if version != "" {
		if err := checkVersion(version); err != nil {
			return nil, err
		}
	}
	disk := fsys.Disk(dir)
	source, err := ops.CheckUpdate(disk, name)
	if err != nil {
		return nil, err
	}
	loc, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	pack, err := fetch(name, source, loc, version)
	if err != nil {
		return nil, err
	}
	if err := ops.UpdatePack(disk, pack); err != nil {
		return nil, err
	}
	return installed(pack), nil
}

// Remove removes the installed pack name.
func Remove(dir, name string) error {
	return ops.RemovePack(fsys.Disk(dir), name)
}

// Install writes every pack in dg.lock at its commit, and returns their names. A pack whose files
// do not match their hash refuses them all.
func Install(dir string) ([]string, error) {
	disk := fsys.Disk(dir)
	locked, err := ops.LockedPacks(disk)
	if err != nil {
		return nil, err
	}
	names := slices.Sorted(maps.Keys(locked))
	packs := make([]ops.Pack, len(names))
	for i, name := range names {
		e := locked[name]
		loc, err := parseSource(e.Source)
		if err != nil {
			return nil, err
		}
		pack, err := withRepo(func(r *repo) (ops.Pack, error) {
			commit, err := r.fetchCommit(loc.url, e.Commit)
			if err != nil {
				return ops.Pack{}, err
			}
			return r.pack(name, e.Source, loc, e.Version, commit)
		})
		if err != nil {
			return nil, err
		}
		if pack.Entry.Hash != e.Hash {
			return nil, fmt.Errorf("the pack %q at %s has the hash %s, not the %s of %s", name, e.Commit, pack.Entry.Hash, e.Hash, project.LockFile)
		}
		packs[i] = pack
	}
	if err := ops.InstallPacks(disk, packs); err != nil {
		return nil, err
	}
	return names, nil
}

// Entry is an entry of the index. It says only where a pack is: a source from here is refused
// or accepted as one from an argument is.
type Entry struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	License     string `json:"license"`
}

// decodeIndex decodes index.json, with what a terminal would act on taken out of its text.
func decodeIndex(data []byte) ([]Entry, error) {
	var index struct {
		Version *int    `json:"version"`
		Packs   []Entry `json:"packs"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("index.json: %w", err)
	}
	switch {
	case index.Version == nil:
		return nil, errors.New("index.json has no version")
	case *index.Version != 1:
		return nil, fmt.Errorf("index.json has version %d; the version is 1. %s", *index.Version, diag.Newer())
	}
	for i := range index.Packs {
		e := &index.Packs[i]
		for _, field := range []*string{&e.Name, &e.Source, &e.Description, &e.License} {
			*field = strings.ReplaceAll(printable(*field), "\n", " ")
		}
	}
	return index.Packs, nil
}

func fetchIndex() ([]Entry, error) {
	loc, err := parseSource(indexSource)
	if err != nil {
		return nil, err
	}
	files, err := withRepo(func(r *repo) (fstest.MapFS, error) {
		commit, err := r.fetchOne(loc.url, "HEAD")
		if err != nil {
			return nil, err
		}
		return r.files(commit, "", func(name string) bool { return name == "index.json" })
	})
	if err != nil {
		return nil, err
	}
	f := files["index.json"]
	if f == nil || !f.Mode.IsRegular() {
		return nil, fmt.Errorf("%s has no file index.json", indexSource)
	}
	return decodeIndex(f.Data)
}

// Search returns the index's entries whose name or description holds text, ignoring case, by name.
func Search(text string) ([]Entry, error) {
	entries, err := fetchIndex()
	if err != nil {
		return nil, err
	}
	return match(entries, text), nil
}

func match(entries []Entry, text string) []Entry {
	text = strings.ToLower(text)
	found := slices.DeleteFunc(slices.Clone(entries), func(e Entry) bool {
		return !strings.Contains(strings.ToLower(e.Name), text) && !strings.Contains(strings.ToLower(e.Description), text)
	})
	slices.SortStableFunc(found, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return found
}
