package pack

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing/fstest"
	"unicode"

	"github.com/nvcnvn/dg/internal/project"
)

// A fetch runs the user's git, with the user's and the system's configuration, which is where
// credentials, url.<base>.insteadOf and core.sshCommand live and which the user trusts; nothing in
// them comes from the repository. It runs in a new bare repository, so that no repository's
// configuration applies, and never checks anything out: files are read from objects, so no
// .gitattributes picks a filter, no link is made and no submodule has a place to go.

// fetchedRef is where a fetch puts the revision it asked for.
const fetchedRef = "refs/dg/fetched"

// gitConfig switches off what the user's configuration could run during a fetch, whatever
// repository it is from.
var gitConfig = []string{
	"-c", "core.hooksPath=" + os.DevNull,
	"-c", "core.fsmonitor=false",
	"-c", "fetch.recurseSubmodules=false",
	"-c", "submodule.recurse=false",
	"-c", "gc.auto=0",
	"-c", "maintenance.auto=false",
}

// gitLocalEnv names what would make git work on another repository than the one it is given, as when
// dg runs from a git hook.
var gitLocalEnv = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_DEFAULT_HASH", "GIT_GRAFT_FILE", "GIT_IMPLICIT_WORK_TREE", "GIT_INDEX_FILE",
	"GIT_NAMESPACE", "GIT_OBJECT_DIRECTORY", "GIT_PREFIX", "GIT_REPLACE_REF_BASE", "GIT_SHALLOW_FILE", "GIT_WORK_TREE",
}

// maxEntries is how many paths a repository may hold, about 100 times the largest pack spec 4.7
// expects; ls-tree of a few kilobytes of trees that share their subtrees can list billions.
const maxEntries = 100_000

// maxPath is the length in bytes of the longest path of a repository, as long as most systems
// can open.
const maxPath = 4096

// repo is a temporary bare repository.
type repo struct {
	dir string
}

// withRepo runs f in a new repository, which is gone when it returns.
func withRepo[T any](f func(*repo) (T, error)) (T, error) {
	var zero T
	dir, err := os.MkdirTemp("", "dg-pack-")
	if err != nil {
		return zero, err
	}
	defer os.RemoveAll(dir)
	r := &repo{dir}
	// An empty template: hooks in the user's template directory would run in this repository. SHA-1
	// whatever the user's default is: a fetch fails on any other format than the remote's.
	if _, err := r.git("", "init", "--quiet", "--bare", "--template=", "--object-format=sha1"); err != nil {
		return zero, err
	}
	return f(r)
}

func (r *repo) command(args ...string) *exec.Cmd {
	cmd := exec.Command("git", append(slices.Clone(gitConfig), args...)...)
	cmd.Dir = r.dir
	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(gitLocalEnv, name)
	})
	cmd.Env = append(cmd.Env,
		"GIT_DIR="+r.dir,
		"GIT_ALLOW_PROTOCOL=https:ssh:file", // also what insteadOf rewrites to: ext:: runs a command
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_TERMINAL_PROMPT=0", // credential helpers and askpass programs still answer
	)
	return cmd
}

func gitError(err error, command string, stderr *bytes.Buffer) error {
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("dg pack needs git on the path to fetch packs")
	}
	return fmt.Errorf("git %s: %s", command, printable(strings.TrimSpace(stderr.String())))
}

func (r *repo) git(stdin string, args ...string) ([]byte, error) {
	cmd := r.command(args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, gitError(err, args[0], &stderr)
	}
	return stdout.Bytes(), nil
}

// splitNUL is a bufio.SplitFunc for NUL-terminated records.
func splitNUL(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// eachEntry calls f on every entry of the tree of commit as ls-tree prints it, reading no more of
// it than the limits allow: what a repository makes it print has no bound of its own.
func (r *repo) eachEntry(commit string, f func(entry string) error) error {
	cmd := r.command("ls-tree", "-r", "-z", "--full-tree", commit)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return gitError(err, "ls-tree", &stderr)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4096), maxPath+len("100644 blob \t")+64)
	scanner.Split(splitNUL)
	refused := func() error {
		for n := 0; scanner.Scan(); n++ {
			if n == maxEntries {
				return fmt.Errorf("the repository holds more than %d paths", maxEntries)
			}
			if err := f(scanner.Text()); err != nil {
				return err
			}
		}
		if errors.Is(scanner.Err(), bufio.ErrTooLong) {
			return fmt.Errorf("the repository holds a path longer than %d bytes", maxPath)
		}
		return scanner.Err()
	}()
	if refused != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return refused
	}
	if err := cmd.Wait(); err != nil {
		return gitError(err, "ls-tree", &stderr)
	}
	return nil
}

// printable drops what a terminal would act on from text that came from outside, keeping lines.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, s)
}

// refs returns the names of the url's tags and branches, as refs/tags/... and refs/heads/.... A
// repository in any format but SHA-1 is refused: a fetch from it would fail.
func (r *repo) refs(url string) ([]string, error) {
	out, err := r.git("", "ls-remote", "--", url, "refs/tags/*", "refs/heads/*")
	if err != nil {
		return nil, err
	}
	var refs []string
	for line := range strings.Lines(string(out)) {
		oid, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if !project.IsCommit(oid) {
			return nil, fmt.Errorf("%s is not a SHA-1 repository, the only format dg fetches", url)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

var releaseRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// highest returns the highest vX.Y.Z tag of refs, or "". Numbers are compared as text of equal
// length, since a tag can hold more digits than an integer.
func highest(refs []string) string {
	best := ""
	var bestParts []string
	for _, ref := range refs {
		tag, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		parts := releaseRE.FindStringSubmatch(tag)
		if parts == nil {
			continue
		}
		if best == "" || slices.CompareFunc(parts[1:], bestParts, func(a, b string) int {
			return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b))
		}) > 0 {
			best, bestParts = tag, parts[1:]
		}
	}
	return best
}

// pickTag returns the tag that version names, by default the highest release, in refs. Only a
// name the user gave or a release's name leaves here, so no tag a repository holds reaches git
// on its own.
func pickTag(refs []string, version, url string) (string, error) {
	switch {
	case version == "":
		tag := highest(refs)
		if tag == "" {
			return "", fmt.Errorf("%s has no vX.Y.Z tag; name a tag or a full commit hash", url)
		}
		return tag, nil
	case slices.Contains(refs, "refs/tags/"+version):
		return version, nil
	case slices.Contains(refs, "refs/heads/"+version):
		return "", fmt.Errorf("%q is a branch of %s, which moves; name a tag or a full commit hash", version, url)
	}
	return "", fmt.Errorf("%s has no tag %q", url, version)
}

// fetchOne fetches one revision, without history, and returns its commit.
func (r *repo) fetchOne(url, refspec string) (string, error) {
	_, err := r.git("", "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", "--depth=1", "--", url, "+"+refspec+":"+fetchedRef)
	if err != nil {
		return "", err
	}
	return r.commit(fetchedRef)
}

// fetchCommit fetches a commit by its hash. A server may refuse one no ref points at, as protocol
// version 0 does by default; then every tag and branch is fetched, with their history, to find it.
func (r *repo) fetchCommit(url, hash string) (string, error) {
	if _, err := r.refs(url); err != nil {
		return "", err
	}
	commit, first := r.fetchOne(url, hash)
	if first != nil {
		if _, err := r.git("", "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", "--", url, "+refs/heads/*:refs/dg/heads/*", "+refs/tags/*:refs/dg/tags/*"); err != nil {
			return "", err
		}
		var err error
		if commit, err = r.commit(hash); err != nil {
			return "", fmt.Errorf("%s has no commit %s that dg can reach: %w", url, hash, first)
		}
	}
	if commit != hash {
		return "", fmt.Errorf("%s is a tag object, not a commit; name the tag, or its commit %s", hash, commit)
	}
	return commit, nil
}

func (r *repo) commit(rev string) (string, error) {
	out, err := r.git("", "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s is not a commit", rev)
	}
	commit := strings.TrimSpace(string(out))
	if !project.IsCommit(commit) {
		return "", fmt.Errorf("git gave %q for a commit", commit)
	}
	return commit, nil
}

// checkPath refuses a path of a pack that the disk could not hold as it is written, that would
// not read back the same on a disk that folds case or normalizes names, or that a terminal would
// show as something else. Being printable ASCII leaves case the only folding left to compare,
// which folded does, by every directory on the way to the path.
func checkPath(rel string, folded map[string]string) error {
	if !fs.ValidPath(rel) || strings.ContainsFunc(rel, func(c rune) bool { return c < ' ' || c > '~' || c == '\\' || c == ':' }) {
		return fmt.Errorf("the path %q is not one a pack can hold, which is printable ASCII without \\ or a colon", rel)
	}
	for p := rel; p != "."; p = path.Dir(p) {
		if strings.EqualFold(path.Base(p), ".git") {
			return fmt.Errorf("the path %q is inside .git", rel)
		}
		lower := strings.ToLower(p)
		if other, ok := folded[lower]; ok && other != p {
			return fmt.Errorf("%s and %s differ only by case, so a disk that ignores case would keep one", other, p)
		}
		folded[lower] = p
	}
	return nil
}

// files returns the files of commit under dir, or of the whole tree if dir is "", for which want
// reports true, by their paths under dir. A symbolic link is an entry with that mode and no data.
// Nothing is checked out, so no path a repository holds reaches the disk from here; it is
// refused here if it could not safely do so later.
func (r *repo) files(commit, dir string, want func(string) bool) (fstest.MapFS, error) {
	files := fstest.MapFS{}
	byBlob := map[string][]string{}
	found := dir == ""
	folded := map[string]string{}
	err := r.eachEntry(commit, func(entry string) error {
		meta, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return fmt.Errorf("git ls-tree gave %q", printable(entry))
		}
		mode, kind, oid := fields[0], fields[1], fields[2]
		if name == dir || strings.HasPrefix(dir, name+"/") {
			if mode == "120000" {
				return fmt.Errorf("%s is a symbolic link", printable(name))
			}
			return fmt.Errorf("%s is not a directory", printable(name))
		}
		rel := name
		if dir != "" {
			if rel, ok = strings.CutPrefix(name, dir+"/"); !ok {
				return nil
			}
		}
		found = true
		if kind != "blob" || !want(rel) {
			return nil
		}
		if err := checkPath(rel, folded); err != nil {
			return err
		}
		if mode == "120000" {
			files[rel] = &fstest.MapFile{Mode: fs.ModeSymlink}
			return nil
		}
		byBlob[oid] = append(byBlob[oid], rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("the repository has no directory %s", dir)
	}
	if len(byBlob) == 0 {
		return files, nil
	}
	// Each blob once, since many paths can share one at no cost to the repository.
	oids := slices.Sorted(maps.Keys(byBlob))
	blobs, err := r.git(strings.Join(oids, "\n")+"\n", "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	for _, oid := range oids {
		header, rest, _ := bytes.Cut(blobs, []byte("\n"))
		fields := strings.Fields(string(header))
		if len(fields) != 3 {
			return nil, fmt.Errorf("git cat-file gave %q for %s", printable(string(header)), oid)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size >= len(rest) {
			return nil, fmt.Errorf("git cat-file gave %q for %s", printable(string(header)), oid)
		}
		for _, name := range byBlob[oid] {
			files[name] = &fstest.MapFile{Data: rest[:size:size], Mode: 0o644}
		}
		blobs = rest[size+1:]
	}
	return files, nil
}
