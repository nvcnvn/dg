package pack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/project"
)

const testSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`

// goodPack is a repository with a pack at its root, and with what a pack does not hold.
var goodPack = map[string]string{
	"spec.hcl":      "kind \"vpc\" {\n  extends = \"group\"\n}\n",
	"style.hcl":     "style \"vpc\" {\n  icon = file(\"icons/vpc.svg\")\n}\n",
	"icons/vpc.svg": testSVG,
	"LICENSE":       "license\n",
	"README.md":     "read me\n",
	"install.sh":    "#!/bin/sh\nexit 1\n",
	"docs/notes.md": "notes\n",
}

// hermetic gives git a configuration of its own, and returns the file that holds it.
func hermetic(t *testing.T) string {
	t.Helper()
	config := filepath.Join(t.TempDir(), "gitconfig")
	write(t, config, "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("HOME", t.TempDir())
	return config
}

func write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "protocol.file.allow=always"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitAll(t *testing.T, dir string) string {
	t.Helper()
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "--quiet", "--allow-empty", "-m", "c")
	return run(t, dir, "rev-parse", "HEAD")
}

// newRepo returns a repository of files with one commit, its directory and its file URL.
func newRepo(t *testing.T, files map[string]string) (dir, url string) {
	t.Helper()
	dir = t.TempDir()
	run(t, dir, "init", "--quiet", "--initial-branch=main")
	for name, content := range files {
		write(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	commitAll(t, dir)
	return dir, "file://" + filepath.ToSlash(dir)
}

func newProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		write(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	return dir
}

// snapshot returns every file under dir, and every directory as an entry of its own.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, name)
		if d.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(name)
		out[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func unchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	if after := snapshot(t, dir); !maps.Equal(before, after) {
		t.Errorf("the project changed:\nbefore %v\nafter  %v", slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after)))
	}
}

func packFiles(t *testing.T, dir, name string) map[string]string {
	t.Helper()
	return snapshot(t, filepath.Join(dir, "packs", name))
}

func mustAdd(t *testing.T, dir, arg, as string) *Installed {
	t.Helper()
	in, err := Add(dir, arg, as)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func mustFail(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got error %v, want one holding %q", err, want)
	}
}

func diagnostics(t *testing.T, dir string) string {
	t.Helper()
	_, diags, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, d := range diags {
		fmt.Fprintln(&b, d)
	}
	return b.String()
}

func TestParseSource(t *testing.T) {
	tests := []struct {
		in, url, dir string
		bad          string
	}{
		{in: "github.com/dg-packs/aws", url: "https://github.com/dg-packs/aws"},
		{in: "example.com:8443/a/b", url: "https://example.com:8443/a/b"},
		{in: "github.com/a/mono//packs/aws", url: "https://github.com/a/mono", dir: "packs/aws"},
		{in: "https://example.com/a.git", url: "https://example.com/a.git"},
		{in: "ssh://git@example.com/a/b", url: "ssh://git@example.com/a/b"},
		{in: "ssh://git@example.com:22/a//b", url: "ssh://git@example.com:22/a", dir: "b"},
		{in: "file:///tmp/a", url: "file:///tmp/a"},
		{in: "file:///tmp/a//b/c", url: "file:///tmp/a", dir: "b/c"},
		{in: "-oProxyCommand=x/y", bad: "starts with -"},
		{in: "--upload-pack=touch/x", bad: "starts with -"},
		{in: "ext::sh -c touch%20x/y", bad: "a space"},
		{in: "ext::sh/x", bad: "not a URL"},
		{in: "git@github.com:a/b", bad: "not a URL"},
		{in: "http://example.com/a", bad: "scheme"},
		{in: "git://example.com/a", bad: "scheme"},
		{in: "ftp://example.com/a", bad: "scheme"},
		{in: "HTTPS://example.com/a", bad: "scheme"},
		{in: "fd::3/a", bad: "not a URL"},
		{in: "ssh://-oProxyCommand=x/a", bad: "invalid host"},
		{in: "https://-x.example.com/a", bad: "invalid host"},
		{in: "https://u:p@example.com/a", bad: "credentials"},
		{in: "https://token@example.com/a", bad: "credentials"},
		{in: "ssh://u:p@example.com/a", bad: "credentials"},
		{in: "file://host/a", bad: "invalid host"},
		{in: "https://example.com", bad: "repository path"},
		{in: "file:///", bad: "repository path"},
		{in: "https://example.com/a?x=1", bad: "query"},
		{in: "https://example.com/a#x", bad: "query"},
		{in: "example.com/a\nb", bad: "control"},
		{in: "example.com/a//..", bad: "directory"},
		{in: "example.com/a//b/../../c", bad: "directory"},
		{in: "example.com/a//", bad: "directory"},
		{in: "example.com/a//.", bad: "directory"},
		{in: "example.com/a//b//c", bad: "directory"},
		{in: "aws", bad: "<host>/<path>"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			loc, err := parseSource(tt.in)
			if tt.bad != "" {
				mustFail(t, err, tt.bad)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if loc.url != tt.url || loc.dir != tt.dir {
				t.Errorf("got %+v", loc)
			}
		})
	}
}

func TestParseAdd(t *testing.T) {
	tests := []struct{ in, source, version, bad string }{
		{in: "aws", source: "aws"},
		{in: "aws@v1.0.0", source: "aws", version: "v1.0.0"},
		{in: "example.com/a@v1.0.0", source: "example.com/a", version: "v1.0.0"},
		{in: "example.com/a//b@release/v1", source: "example.com/a//b", version: "release/v1"},
		{in: "ssh://git@example.com/a", source: "ssh://git@example.com/a"},
		{in: "ssh://git@example.com/a@v1", source: "ssh://git@example.com/a", version: "v1"},
		{in: "example.com/a@-x", bad: "not a tag"},
		{in: "example.com/a@--upload-pack=touch", bad: "not a tag"},
		{in: "example.com/a@", bad: "not a tag"},
		{in: "example.com/a@a:b", bad: "not a tag"},
		{in: "example.com/a@a..b", bad: "not a tag"},
		{in: "example.com/a@HEAD^", bad: "not a tag"},
		{in: "-x", bad: "neither"},
		{in: "", bad: "neither"},
		{in: "ext::sh", bad: "neither"},
		{in: "a b", bad: "neither"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			source, version, err := parseAdd(tt.in)
			if tt.bad != "" {
				mustFail(t, err, tt.bad)
				return
			}
			if err != nil || source != tt.source || version != tt.version {
				t.Errorf("got %q, %q, %v", source, version, err)
			}
		})
	}
}

func TestHighest(t *testing.T) {
	tests := []struct {
		refs []string
		want string
	}{
		{nil, ""},
		{[]string{"refs/heads/v9.9.9", "refs/tags/latest", "refs/tags/v1.0", "refs/tags/1.0.0", "refs/tags/v01.0.0", "refs/tags/v1.0.0-rc1"}, ""},
		{[]string{"refs/tags/v1.9.0", "refs/tags/v1.10.0", "refs/tags/v1.2.0"}, "v1.10.0"},
		{[]string{"refs/tags/v1.0.9", "refs/tags/v1.0.10", "refs/tags/v0.99.99"}, "v1.0.10"},
		{[]string{"refs/tags/v99999999999999999999999.0.0", "refs/tags/v2.0.0"}, "v99999999999999999999999.0.0"},
		{[]string{"refs/tags/--upload-pack=x", "refs/tags/v1.0.0"}, "v1.0.0"},
	}
	for _, tt := range tests {
		if got := highest(tt.refs); got != tt.want {
			t.Errorf("highest(%v) = %q, want %q", tt.refs, got, tt.want)
		}
	}
}

// TestVersions covers how a version resolves, against a repository.
func TestVersions(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	first := run(t, repo, "rev-parse", "HEAD")
	run(t, repo, "tag", "v1.2.0")
	run(t, repo, "tag", "-a", "-m", "annotated", "v1.9.0")
	tagObject := run(t, repo, "rev-parse", "v1.9.0")
	write(t, filepath.Join(repo, "spec.hcl"), "kind \"vpc2\" {}\n")
	tenth := commitAll(t, repo)
	run(t, repo, "tag", "v1.10.0")
	run(t, repo, "tag", "v1.10.0-rc1")
	run(t, repo, "branch", "feature")
	run(t, repo, "update-ref", "refs/tags/--upload-pack=touch", "HEAD")

	tests := []struct {
		name, arg    string
		version, bad string
		commit       string
	}{
		{name: "highest", arg: url, version: "v1.10.0", commit: tenth},
		{name: "tag", arg: url + "@v1.2.0", version: "v1.2.0", commit: first},
		{name: "annotated tag", arg: url + "@v1.9.0", version: "v1.9.0", commit: first},
		{name: "commit", arg: url + "@" + first, version: first, commit: first},
		{name: "branch", arg: url + "@feature", bad: "is a branch"},
		{name: "main", arg: url + "@main", bad: "is a branch"},
		{name: "no such tag", arg: url + "@v3.0.0", bad: "no tag"},
		{name: "short hash", arg: url + "@" + first[:12], bad: "no tag"},
		{name: "no such commit", arg: url + "@" + strings.Repeat("a", 40), bad: "no commit"},
		{name: "hostile tag", arg: url + "@--upload-pack=touch", bad: "not a tag"},
		{name: "a tag object's hash", arg: url + "@" + tagObject, bad: "is a tag object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t, nil)
			_, err := Add(dir, tt.arg, "aws")
			if tt.bad != "" {
				mustFail(t, err, tt.bad)
				if entries, _ := os.ReadDir(dir); len(entries) != 0 {
					t.Errorf("left %v", entries)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			lock := readLock(t, dir)
			if e := lock.Packs["aws"]; e.Version != tt.version || e.Commit != tt.commit || e.Source != url {
				t.Errorf("got %+v", e)
			}
		})
	}
}

func readLock(t *testing.T, dir string) *project.Lock {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, project.LockFile))
	if err != nil {
		t.Fatal(err)
	}
	lock, diags := project.DecodeLock(data, project.LockFile)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return lock
}

func TestNoReleaseTag(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "nightly")
	_, err := Add(newProject(t, nil), url, "aws")
	mustFail(t, err, "no vX.Y.Z tag")
}

// TestTagAndCommit is FR-21's AC: a tag and its commit write the same pack files.
func TestTagAndCommit(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	commit := run(t, repo, "rev-parse", "HEAD")
	run(t, repo, "tag", "v1.0.0")
	byTag, byCommit := newProject(t, nil), newProject(t, nil)
	mustAdd(t, byTag, url, "aws")
	mustAdd(t, byCommit, url+"@"+commit, "aws")
	if a, b := packFiles(t, byTag, "aws"), packFiles(t, byCommit, "aws"); !maps.Equal(a, b) {
		t.Errorf("%v\n%v", a, b)
	}
	if a, b := readLock(t, byTag).Packs["aws"], readLock(t, byCommit).Packs["aws"]; a.Commit != b.Commit || a.Hash != b.Hash {
		t.Errorf("%+v\n%+v", a, b)
	}
	if diags := diagnostics(t, byTag); strings.Contains(diags, "error") || strings.Contains(diags, "Pack changed") {
		t.Error(diags)
	}
}

// TestUnadvertisedCommit fetches a commit no ref points at from a server that refuses that, which
// is protocol version 0 by default.
func TestUnadvertisedCommit(t *testing.T) {
	config := hermetic(t)
	write(t, config, "[protocol]\n\tversion = 0\n")
	repo, url := newRepo(t, goodPack)
	old := run(t, repo, "rev-parse", "HEAD")
	write(t, filepath.Join(repo, "spec.hcl"), "kind \"new\" {}\n")
	commitAll(t, repo)
	dir := newProject(t, nil)
	mustAdd(t, dir, url+"@"+old, "aws")
	if got := readLock(t, dir).Packs["aws"].Commit; got != old {
		t.Errorf("got %s", got)
	}
	_, err := Add(newProject(t, nil), url+"@"+strings.Repeat("b", 40), "aws")
	mustFail(t, err, "no commit")
}

func TestSubdirectory(t *testing.T) {
	hermetic(t)
	files := map[string]string{"packs/aws/spec.hcl": "kind \"a\" {}\n", "packs/aws/LICENSE": "l", "spec.hcl": "kind \"root\" {}\n", "file": "x"}
	repo, url := newRepo(t, files)
	if err := errors.Join(os.Symlink("packs", filepath.Join(repo, "link")), os.Symlink("aws", filepath.Join(repo, "packs", "here"))); err != nil {
		t.Skip(err)
	}
	commitAll(t, repo)
	run(t, repo, "tag", "v1.0.0")

	dir := newProject(t, nil)
	mustAdd(t, dir, url+"//packs/aws", "")
	if got, want := slices.Sorted(maps.Keys(packFiles(t, dir, "aws"))), []string{"./", "LICENSE", "spec.hcl"}; !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}

	for _, tt := range []struct{ sub, bad string }{
		{"link/aws", "symbolic link"},
		{"link", "symbolic link"},
		{"packs/here", "symbolic link"},
		{"file", "not a directory"},
		{"file/x", "not a directory"},
		{"packs/gone", "no directory"},
		{"packs", "neither spec.hcl nor style.hcl"},
	} {
		t.Run(tt.sub, func(t *testing.T) {
			dir := newProject(t, nil)
			_, err := Add(dir, url+"//"+tt.sub, "aws")
			mustFail(t, err, tt.bad)
			unchanged(t, dir, map[string]string{"./": ""})
		})
	}
}

// TestNothingRuns is FR-21's AC: a repository with a hook, a submodule and a script runs none of
// them, and only the files of section 4.7 are copied, with no mode of their own. The hooks and
// filter here are the user's, which a repository's .gitattributes names and any fetch would reach;
// plain git runs them, so a pass here means dg stopped them.
func TestNothingRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hooks are shell scripts")
	}
	config := hermetic(t)
	scratch := t.TempDir()
	ran := filepath.Join(scratch, "ran")
	script := filepath.Join(scratch, "script")
	write(t, script, "#!/bin/sh\necho \"$0 $*\" >> "+ran+"\ncat\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(scratch, "template", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, hook := range []string{"post-checkout", "reference-transaction", "post-merge", "post-index-change", "fsmonitor-watchman"} {
		if err := os.Symlink(script, filepath.Join(hooks, hook)); err != nil {
			t.Fatal(err)
		}
	}

	_, sub := newRepo(t, map[string]string{"sub.svg": testSVG})
	files := maps.Clone(goodPack)
	files[".gitattributes"] = "* filter=evil text eol=crlf\n"
	files[".gitmodules.note"] = ""
	files["run.sh"] = "#!/bin/sh\ntouch " + ran + "\n"
	repo, url := newRepo(t, files)
	if err := os.Chmod(filepath.Join(repo, "icons", "vpc.svg"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "submodule", "add", "--quiet", sub, "modules/sub")
	for _, hook := range []string{"post-checkout", "post-commit", "pre-commit", "post-update", "pre-receive", "update"} {
		write(t, filepath.Join(repo, ".git", "hooks", hook), "#!/bin/sh\ntouch "+ran+"\n")
		if err := os.Chmod(filepath.Join(repo, ".git", "hooks", hook), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	commitAll(t, repo)
	run(t, repo, "tag", "v1.0.0")
	os.Remove(ran)
	run(t, repo, "config", "core.fsmonitor", script)
	run(t, repo, "config", "core.sshCommand", script)
	run(t, repo, "config", "uploadpack.packObjectsHook", script)

	write(t, config, fmt.Sprintf(`[core]
	hooksPath = %[1]s
	fsmonitor = %[2]s
[init]
	templateDir = %[3]s
[filter "evil"]
	smudge = %[2]s
	required = true
[submodule]
	recurse = true
[fetch]
	recurseSubmodules = true
[protocol "file"]
	allow = always
`, hooks, script, filepath.Dir(hooks)))

	clone := filepath.Join(t.TempDir(), "clone")
	_ = exec.Command("git", "clone", "--quiet", "--recurse-submodules", url, clone).Run()
	if err := os.Remove(ran); err != nil {
		t.Fatal("plain git ran none of what this test stops, so it proves nothing")
	}

	dir := newProject(t, nil)
	mustAdd(t, dir, url, "aws")
	if data, err := os.ReadFile(ran); err == nil {
		t.Errorf("ran: %s", data)
	}
	want := []string{"./", "LICENSE", "README.md", "icons/", "icons/vpc.svg", "spec.hcl", "style.hcl"}
	if got := slices.Sorted(maps.Keys(packFiles(t, dir, "aws"))); !slices.Equal(got, want) {
		t.Errorf("copied %v", got)
	}
	info, err := os.Stat(filepath.Join(dir, "packs", "aws", "icons", "vpc.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", info.Mode())
	}
	if got := packFiles(t, dir, "aws")["icons/vpc.svg"]; got != testSVG {
		t.Errorf("a filter or eol reached %q", got)
	}
}

// TestGitEnvironment checks that what a git hook sets in the environment of dg does not turn the
// fetch on the repository it ran in, and that the configuration a fetch needs from the user
// survives: url.<base>.insteadOf, which also lets the index be redirected in tests, and
// core.sshCommand.
func TestGitEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake ssh is a shell script")
	}
	config := hermetic(t)
	repo, _ := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")

	t.Run("a hook's environment", func(t *testing.T) {
		other, _ := newRepo(t, nil)
		t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
		t.Setenv("GIT_WORK_TREE", other)
		t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
		before := snapshot(t, other)
		dir := newProject(t, nil)
		mustAdd(t, dir, "file://"+repo, "aws")
		unchanged(t, other, before)
	})
	t.Run("insteadOf", func(t *testing.T) {
		write(t, config, fmt.Sprintf("[url %q]\n\tinsteadOf = https://git.example.com/a\n", "file://"+repo))
		mustAdd(t, newProject(t, nil), "git.example.com/a", "aws")
	})
	t.Run("insteadOf to a program", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "ran")
		write(t, config, fmt.Sprintf("[url %q]\n\tinsteadOf = https://evil.example.com/\n[protocol]\n\tallow = always\n", "ext::sh -c touch% "+marker+" #"))
		_, err := Add(newProject(t, nil), "evil.example.com/a", "")
		if err == nil {
			t.Fatal("no error")
		}
		if _, err := os.Stat(marker); err == nil {
			t.Error("the ext transport ran")
		}
	})
	t.Run("sshCommand", func(t *testing.T) {
		scratch := t.TempDir()
		marker := filepath.Join(scratch, "ran")
		fake := filepath.Join(scratch, "ssh")
		write(t, fake, "#!/bin/sh\ntouch "+marker+"\nexit 1\n")
		if err := os.Chmod(fake, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, config, fmt.Sprintf("[core]\n\tsshCommand = %s\n", fake))
		_, err := Add(newProject(t, nil), "ssh://git@example.com/a/b", "")
		if err == nil {
			t.Fatal("no error")
		}
		if _, err := os.Stat(marker); err != nil {
			t.Error("the user's core.sshCommand did not run")
		}
	})
}

func TestSymbolicLinks(t *testing.T) {
	hermetic(t)
	secret := filepath.Join(t.TempDir(), "secret.svg")
	write(t, secret, "secret")
	for _, tt := range []struct {
		name, link string
		refused    bool
	}{
		{"svg", "icons/evil.svg", true},
		{"spec", "README.md.link", true},
		{"license", "LICENSE-link", true},
		{"elsewhere", "docs/link.txt", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, url := newRepo(t, goodPack)
			if err := os.Symlink(secret, filepath.Join(repo, filepath.FromSlash(tt.link))); err != nil {
				t.Skip(err)
			}
			commitAll(t, repo)
			run(t, repo, "tag", "v1.0.0")
			dir := newProject(t, nil)
			_, err := Add(dir, url, "aws")
			if !tt.refused {
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := packFiles(t, dir, "aws")["docs/link.txt"]; ok {
					t.Error("copied a file outside the set")
				}
				return
			}
			mustFail(t, err, "symbolic link")
			unchanged(t, dir, map[string]string{"./": ""})
		})
	}
}

func TestCaseCollision(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, map[string]string{"spec.hcl": "", "a.svg": testSVG})
	blob := run(t, repo, "rev-parse", "HEAD:a.svg")
	run(t, repo, "update-index", "--add", "--cacheinfo", "100644,"+blob+",A.svg")
	run(t, repo, "commit", "--quiet", "-m", "c")
	run(t, repo, "tag", "v1.0.0")
	_, err := Add(newProject(t, nil), url, "aws")
	mustFail(t, err, "differ only by case")
}

// TestUnsafePaths fetches trees that git add cannot make, or that a disk here could not hold.
func TestUnsafePaths(t *testing.T) {
	hermetic(t)
	for _, tt := range []struct{ path, bad string }{
		{"../evil.svg", "not one a pack can hold"},
		{"a/../../evil.svg", "not one a pack can hold"},
		{`a\b.svg`, "not one a pack can hold"},
		{"a:b.svg", "not one a pack can hold"},
		{"a\x01b.svg", "not one a pack can hold"},
		{"a\x7fb.svg", "not one a pack can hold"},
		{"caf\u00e9.svg", "not one a pack can hold"},
		{"a\u202eb.svg", "not one a pack can hold"},
		{"README-\u65e5\u672c.md", "not one a pack can hold"},
		{".git/config.svg", "inside .git"},
		{".GIT/config.svg", "inside .git"},
		{"a/.Git/b.svg", "inside .git"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			repo, url := newRepo(t, map[string]string{"spec.hcl": ""})
			spec := run(t, repo, "rev-parse", "HEAD:spec.hcl")
			tree := pathTree(t, repo, spec, strings.Split(tt.path, "/"))
			commit := run(t, repo, "commit-tree", "-m", "c", tree)
			run(t, repo, "tag", "v1.0.0", commit)
			dir := newProject(t, nil)
			_, err := Add(dir, url, "aws")
			mustFail(t, err, tt.bad)
			unchanged(t, dir, map[string]string{"./": ""})
		})
	}
}

func mktree(t *testing.T, repo string, lines ...string) string {
	t.Helper()
	cmd := exec.Command("git", "mktree", "--missing")
	cmd.Dir, cmd.Stdin = repo, strings.NewReader(strings.Join(lines, "\n")+"\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// pathTree returns a tree that holds spec.hcl and a file by the path segments, as they are.
func pathTree(t *testing.T, repo, blob string, segments []string) string {
	t.Helper()
	entry := "100644 blob " + blob + "\t" + segments[len(segments)-1]
	for i := len(segments) - 2; i >= 0; i-- {
		entry = "040000 tree " + mktree(t, repo, entry) + "\t" + segments[i]
	}
	return mktree(t, repo, entry, "100644 blob "+blob+"\tspec.hcl")
}

func TestCaseCollisionInDirectories(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, map[string]string{"spec.hcl": ""})
	blob := run(t, repo, "rev-parse", "HEAD:spec.hcl")
	file := "100644 blob " + blob + "\ta.svg"
	sub := mktree(t, repo, file)
	tree := mktree(t, repo, "040000 tree "+sub+"\tIcons", "040000 tree "+sub+"\ticons", "100644 blob "+blob+"\tspec.hcl")
	run(t, repo, "tag", "v1.0.0", run(t, repo, "commit-tree", "-m", "c", tree))
	_, err := Add(newProject(t, nil), url, "aws")
	mustFail(t, err, "differ only by case")
}

// TestTreeBomb fetches a repository of a few trees that list ten million paths, each of them one
// a pack holds, and checks that the refusal comes before dg has read them. Reading them all would
// allocate more than a gigabyte.
func TestTreeBomb(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, map[string]string{"spec.hcl": ""})
	blob := run(t, repo, "rev-parse", "HEAD:spec.hcl")
	var files []string
	for i := range 10 {
		files = append(files, fmt.Sprintf("100644 blob %s\t%d.svg", blob, i))
	}
	tree := mktree(t, repo, files...)
	for range 6 {
		var dirs []string
		for i := range 10 {
			dirs = append(dirs, fmt.Sprintf("040000 tree %s\t%d", tree, i))
		}
		tree = mktree(t, repo, dirs...)
	}
	run(t, repo, "tag", "v1.0.0", run(t, repo, "commit-tree", "-m", "c", tree))

	dir := newProject(t, nil)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := Add(dir, url, "aws")
	runtime.ReadMemStats(&after)
	mustFail(t, err, "more than 100000 paths")
	unchanged(t, dir, map[string]string{"./": ""})
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 256<<20 {
		t.Errorf("allocated %d MiB", allocated>>20)
	}
}

func TestLongPath(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, map[string]string{"spec.hcl": ""})
	blob := run(t, repo, "rev-parse", "HEAD:spec.hcl")
	segments := slices.Repeat([]string{strings.Repeat("a", 200)}, 21)
	segments = append(segments, "a.txt")
	tree := pathTree(t, repo, blob, segments)
	run(t, repo, "tag", "v1.0.0", run(t, repo, "commit-tree", "-m", "c", tree))
	_, err := Add(newProject(t, nil), url, "aws")
	mustFail(t, err, "longer than 4096")
}

func TestObjectFormat(t *testing.T) {
	hermetic(t)
	t.Run("a SHA-256 repository", func(t *testing.T) {
		repo := t.TempDir()
		run(t, repo, "init", "--quiet", "--object-format=sha256")
		write(t, filepath.Join(repo, "spec.hcl"), "")
		hash := commitAll(t, repo)
		run(t, repo, "tag", "v1.0.0")
		url := "file://" + filepath.ToSlash(repo)
		for _, arg := range []string{url, url + "@v1.0.0", url + "@" + hash, url + "@" + strings.Repeat("a", 40)} {
			_, err := Add(newProject(t, nil), arg, "aws")
			mustFail(t, err, "not a SHA-1 repository")
		}
	})
	t.Run("a user's default of SHA-256", func(t *testing.T) {
		repo, url := newRepo(t, goodPack)
		commit := run(t, repo, "rev-parse", "HEAD")
		run(t, repo, "tag", "v1.0.0")
		t.Setenv("GIT_DEFAULT_HASH", "sha256")
		mustAdd(t, newProject(t, nil), url, "aws")
		mustAdd(t, newProject(t, nil), url+"@"+commit, "aws")
	})
}

// TestUntouched covers what must leave the project byte-identical.
func TestUntouched(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")
	_, empty := newRepo(t, map[string]string{"README.md": "no pack"})
	emptyDir := strings.TrimPrefix(empty, "file://")
	run(t, emptyDir, "tag", "v1.0.0")

	const local = "project \"p\" {\n  packs = [\"packs/local\"]\n}\n"
	tests := []struct {
		name  string
		files map[string]string
		arg   string
		as    string
		bad   string
	}{
		{name: "failed fetch", arg: url + "-gone", as: "aws", bad: "git"},
		{name: "refused pack", arg: empty, as: "aws", bad: "neither spec.hcl nor style.hcl"},
		{name: "read-only dg.hcl", files: map[string]string{"dg.hcl": "project \"p\" {\n  read_only = [\"dg.hcl\"]\n}\n"}, arg: url, as: "aws", bad: "dg.hcl is read-only"},
		{name: "read-only dg.lock", files: map[string]string{"dg.hcl": "project \"p\" {\n  read_only = [\"dg.lock\"]\n}\n"}, arg: url, as: "aws", bad: "dg.lock is read-only"},
		{name: "read-only pack", files: map[string]string{"dg.hcl": "project \"p\" {\n  read_only = [\"packs/*/spec.hcl\"]\n}\n"}, arg: url, as: "aws", bad: "packs/aws/spec.hcl is read-only"},
		{name: "dg.hcl does not parse", files: map[string]string{"dg.hcl": "project {"}, arg: url, as: "aws", bad: "dg.hcl has errors"},
		{name: "read_only does not decode", files: map[string]string{"dg.hcl": "project \"p\" {\n  read_only = 3\n}\n"}, arg: url, as: "aws", bad: "read_only may be incomplete"},
		{name: "dg.lock does not decode", files: map[string]string{"dg.lock": "version: 2\n"}, arg: url, as: "aws", bad: "dg.lock has errors"},
		{name: "taken", files: map[string]string{"dg.hcl": "project \"p\" {\n  pack \"aws\" {\n    source  = \"x/y\"\n    version = \"v1\"\n  }\n}\n"}, arg: url, as: "aws", bad: "already installed"},
		{name: "taken, by case", files: map[string]string{"dg.hcl": "project \"p\" {\n  pack \"aws\" {\n    source  = \"x/y\"\n    version = \"v1\"\n  }\n}\n"}, arg: url, as: "AWS", bad: "differs only by case"},
		{name: "locked, by case", files: map[string]string{"dg.lock": "version: 1\npacks:\n  aws:\n    source: x/y\n    version: v1\n    commit: " + strings.Repeat("a", 40) + "\n    hash: sha256:" + strings.Repeat("a", 64) + "\n"}, arg: url, as: "AWS", bad: "differs only by case"},
		{name: "local pack", files: map[string]string{"dg.hcl": local, "packs/local/spec.hcl": ""}, arg: url, as: "local", bad: "must not hold a local pack"},
		{name: "local pack, by case", files: map[string]string{"dg.hcl": local, "packs/local/spec.hcl": ""}, arg: url, as: "LOCAL", bad: "must not hold a local pack"},
		{name: "a directory", files: map[string]string{"packs/aws/notes": ""}, arg: url, as: "aws", bad: "must not hold a local pack"},
		{name: "generic", arg: url, as: "generic", bad: "built-in"},
		{name: "not a simple ID", arg: url, as: "a/../b", bad: "simple ID"},
		{name: "no name", arg: url + "/x.y", bad: "simple ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newProject(t, tt.files)
			before := snapshot(t, dir)
			_, err := Add(dir, tt.arg, tt.as)
			mustFail(t, err, tt.bad)
			unchanged(t, dir, before)
		})
	}
}

// TestRepair is the use of dg pack on a project dg check rejects.
func TestRepair(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")
	dir := newProject(t, map[string]string{"dg.hcl": "project \"p\" {}\n", "model.hcl": "node \"n\" {\n  kind = \"aws-vpc\"\n}\n"})
	mustAdd(t, dir, url, "aws")
	config, lock := read(t, dir, "dg.hcl"), read(t, dir, "dg.lock")
	pack := packFiles(t, dir, "aws")

	remove := func() {
		t.Helper()
		if err := os.RemoveAll(filepath.Join(dir, "packs")); err != nil {
			t.Fatal(err)
		}
	}
	remove()
	if !strings.Contains(diagnostics(t, dir), "Pack not installed") {
		t.Fatal(diagnostics(t, dir))
	}
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(pack, packFiles(t, dir, "aws")) || read(t, dir, "dg.hcl") != config || read(t, dir, "dg.lock") != lock {
		t.Error("install did not restore the pack, or changed dg.hcl or dg.lock")
	}

	write(t, filepath.Join(dir, "packs", "aws", "spec.hcl"), "kind \"edited\" {}\n")
	write(t, filepath.Join(dir, "packs", "aws", "stray.txt"), "stray")
	if err := os.Symlink("../..", filepath.Join(dir, "packs", "aws", "link.svg")); err != nil {
		t.Skip(err)
	}
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(pack, packFiles(t, dir, "aws")) {
		t.Errorf("install did not repair the pack: %v", packFiles(t, dir, "aws"))
	}
	if diags := diagnostics(t, dir); strings.Contains(diags, "error") {
		t.Error(diags)
	}

	write(t, filepath.Join(dir, "packs", "aws", "spec.hcl"), "{")
	remove()
	if err := Remove(dir, "aws"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "dg.hcl"); got != "project \"p\" {}\n" {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "dg.lock")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("dg.lock is left")
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestConfigKept checks that dg pack leaves a dg.hcl it has no edit for byte for byte, in a form
// that hclwrite does not reproduce, and so that read_only on it does not stand in the way.
func TestConfigKept(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")
	block := "\n  pack \"aws\" {\n    source  = \"" + url + "\"\n    version = \"v1.0.0\"\n  }\n}\n"
	for _, tt := range []struct{ name, config string }{
		{"a tab", "project \"p\" {\n\tgrid = 10\n" + block},
		{"a tab and read_only", "project \"p\" {\n\tread_only = [\"dg.hcl\"]\n" + block},
		{"a byte order mark", "\ufeffproject \"p\" {\n  grid = 10\n" + block},
	} {
		for action, do := range map[string]func(string) error{
			"install":              func(dir string) error { _, err := Install(dir); return err },
			"update, same version": func(dir string) error { _, err := Update(dir, "aws", "v1.0.0"); return err },
		} {
			t.Run(tt.name+"/"+action, func(t *testing.T) {
				dir := newProject(t, nil)
				mustAdd(t, dir, url, "aws")
				write(t, filepath.Join(dir, "dg.hcl"), tt.config)
				if err := do(dir); err != nil {
					t.Fatal(err)
				}
				if got := read(t, dir, "dg.hcl"); got != tt.config {
					t.Errorf("changed dg.hcl to %q", got)
				}
			})
		}
	}
}

// TestReadOnlyOldFiles checks that read_only protects a file only the old version has.
func TestReadOnlyOldFiles(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")
	if err := os.Remove(filepath.Join(repo, "icons", "vpc.svg")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, "style.hcl"), "")
	commitAll(t, repo)
	run(t, repo, "tag", "v1.1.0")
	dir := newProject(t, map[string]string{"dg.hcl": "project \"p\" {\n  read_only = [\"packs/aws/icons/*\"]\n}\n"})
	if _, err := Add(dir, url+"@v1.0.0", "aws"); err == nil {
		t.Fatal("add wrote a read-only file")
	}
	write(t, filepath.Join(dir, "dg.hcl"), "project \"p\" {}\n")
	mustAdd(t, dir, url+"@v1.0.0", "aws")
	config := strings.Replace(read(t, dir, "dg.hcl"), "{\n", "{\n  read_only = [\"packs/aws/icons/*\"]\n\n", 1)
	write(t, filepath.Join(dir, "dg.hcl"), config)
	before := snapshot(t, dir)
	_, err := Update(dir, "aws", "v1.1.0")
	mustFail(t, err, "packs/aws/icons/vpc.svg is read-only")
	unchanged(t, dir, before)
	mustFail(t, Remove(dir, "aws"), "packs/aws/icons/vpc.svg is read-only")
	unchanged(t, dir, before)
}

func TestInstallRefuses(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")
	base := newProject(t, nil)
	mustAdd(t, base, url, "aws")
	lock := read(t, base, "dg.lock")
	entry := readLock(t, base).Packs["aws"]

	hash := strings.Replace(lock, entry.Hash, "sha256:"+strings.Repeat("0", 64), 1)
	dir := newProject(t, map[string]string{"dg.hcl": read(t, base, "dg.hcl"), "dg.lock": hash, "packs/aws/spec.hcl": "kept"})
	before := snapshot(t, dir)
	_, err := Install(dir)
	mustFail(t, err, "has the hash")
	unchanged(t, dir, before)

	dir = newProject(t, map[string]string{"dg.hcl": "project \"p\" {\n  packs = [\"packs/aws\"]\n}\n", "dg.lock": lock, "packs/aws/spec.hcl": "kept"})
	before = snapshot(t, dir)
	_, err = Install(dir)
	mustFail(t, err, "local pack")
	unchanged(t, dir, before)

	dir = newProject(t, map[string]string{"dg.hcl": "project \"p\" {\n  read_only = [\"packs/aws/spec.hcl\"]\n}\n", "dg.lock": lock, "packs/aws/spec.hcl": "kept"})
	before = snapshot(t, dir)
	_, err = Install(dir)
	mustFail(t, err, "packs/aws/spec.hcl is read-only")
	unchanged(t, dir, before)

	hostile := strings.ReplaceAll(lock, url, "--upload-pack=touch/x")
	dir = newProject(t, map[string]string{"dg.lock": hostile})
	before = snapshot(t, dir)
	_, err = Install(dir)
	mustFail(t, err, "starts with -")
	unchanged(t, dir, before)

	dir = newProject(t, map[string]string{"dg.hcl": "project \"p\" {\n  read_only = [\"dg.hcl\", \"dg.lock\"]\n}\n", "dg.lock": lock})
	if _, err := Install(dir); err != nil {
		t.Errorf("install writes neither read-only file: %v", err)
	}
}

// TestUpdate is FR-21's AC: update writes no model, view or layout file, leaves no file of the
// old version, and a kind it removed is an error at each use.
func TestUpdate(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, goodPack)
	run(t, repo, "tag", "v1.0.0")
	write(t, filepath.Join(repo, "spec.hcl"), "kind \"subnet\" {}\n")
	write(t, filepath.Join(repo, "style.hcl"), "style \"subnet\" {}\n")
	if err := os.Remove(filepath.Join(repo, "icons", "vpc.svg")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, "shapes", "s.svg"), testSVG)
	commitAll(t, repo)
	run(t, repo, "tag", "v1.1.0")

	dir := newProject(t, map[string]string{
		"model.hcl":              "node \"a\" {\n  kind = \"aws-vpc\"\n}\nnode \"b\" {\n  kind = \"aws-vpc\"\n}\n",
		"views.hcl":              "view \"main\" {\n  include = [all]\n}\n",
		"views/main.layout.yaml": "version: 1\n",
	})
	mustAdd(t, dir, url+"@v1.0.0", "aws")
	before := snapshot(t, dir)
	if _, err := Update(dir, "aws", ""); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, dir)
	all := maps.Clone(before)
	maps.Copy(all, after)
	var changed []string
	for _, name := range slices.Sorted(maps.Keys(all)) {
		_, was := before[name]
		_, is := after[name]
		if was != is || after[name] != before[name] {
			changed = append(changed, name)
		}
	}
	want := []string{"dg.hcl", "dg.lock", "packs/aws/icons/", "packs/aws/icons/vpc.svg", "packs/aws/shapes/", "packs/aws/shapes/s.svg", "packs/aws/spec.hcl", "packs/aws/style.hcl"}
	if !slices.Equal(changed, want) {
		t.Errorf("changed %v, want %v", changed, want)
	}
	if !strings.Contains(read(t, dir, "dg.hcl"), `version = "v1.1.0"`) {
		t.Error(read(t, dir, "dg.hcl"))
	}
	if e := readLock(t, dir).Packs["aws"]; e.Version != "v1.1.0" {
		t.Errorf("%+v", e)
	}
	diags := diagnostics(t, dir)
	if strings.Count(diags, `aws-vpc`) != 2 || !strings.Contains(diags, "model.hcl:1:") || !strings.Contains(diags, "model.hcl:4:") {
		t.Errorf("want an error at each use:\n%s", diags)
	}

	if _, err := Update(dir, "aws", "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, dir, "dg.hcl"), `version = "v1.0.0"`) {
		t.Error(read(t, dir, "dg.hcl"))
	}
	fork, forkURL := newRepo(t, map[string]string{"spec.hcl": "kind \"fork\" {}\n"})
	run(t, fork, "tag", "v2.0.0")
	write(t, filepath.Join(dir, "dg.hcl"), strings.Replace(read(t, dir, "dg.hcl"), url, forkURL, 1))
	if _, err := Update(dir, "aws", ""); err != nil {
		t.Fatal(err)
	}
	if e := readLock(t, dir).Packs["aws"]; e.Source != forkURL || e.Version != "v2.0.0" {
		t.Errorf("update did not follow the block's source: %+v", e)
	}
	for _, tt := range []struct{ name, version, bad string }{
		{"gone", "", "no pack"},
		{"../aws", "", "simple ID"},
		{"aws", "--upload-pack=x", "not a tag"},
		{"aws", "main", "is a branch"},
	} {
		before := snapshot(t, dir)
		_, err := Update(dir, tt.name, tt.version)
		mustFail(t, err, tt.bad)
		unchanged(t, dir, before)
	}
}

func TestIndex(t *testing.T) {
	config := hermetic(t)
	pack, packURL := newRepo(t, goodPack)
	run(t, pack, "tag", "v1.0.0")
	index := func(t *testing.T, content string) {
		t.Helper()
		data, err := json.Marshal(map[string]any{"version": 1, "packs": []map[string]string{
			{"name": "aws", "source": packURL, "description": "AWS services and resources", "license": "see NOTICE"},
			{"name": "gcp", "source": "--upload-pack=touch x/y", "description": "Google \x1b[31mcloud"},
			{"name": "ext", "source": "ext::sh -c touch% x"},
			{"name": "loop", "source": "aws"},
			{"name": "http", "source": "http://example.com/a"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if content == "" {
			content = string(data)
		}
		_, url := newRepo(t, map[string]string{"index.json": content})
		write(t, config, fmt.Sprintf("[url %q]\n\tinsteadOf = https://github.com/dg-packs/index\n", url))
	}

	t.Run("lookup", func(t *testing.T) {
		index(t, "")
		dir := newProject(t, nil)
		mustAdd(t, dir, "aws@v1.0.0", "aws")
		if e := readLock(t, dir).Packs["aws"]; e.Source != packURL || e.Version != "v1.0.0" {
			t.Errorf("%+v", e)
		}
		if !strings.Contains(read(t, dir, "dg.hcl"), packURL) {
			t.Error("dg.hcl does not hold the source")
		}
		mustAdd(t, dir, "aws", "other")
	})
	t.Run("hostile entries", func(t *testing.T) {
		index(t, "")
		for _, tt := range []struct{ name, bad string }{
			{"gcp", "starts with -"},
			{"ext", "a space"},
			{"loop", "<host>/<path>"},
			{"http", "scheme"},
			{"azure", "no pack"},
		} {
			dir := newProject(t, nil)
			_, err := Add(dir, tt.name, "")
			mustFail(t, err, tt.bad)
			unchanged(t, dir, map[string]string{"./": ""})
		}
	})
	t.Run("search", func(t *testing.T) {
		index(t, "")
		found, err := Search("CLOUD")
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].Name != "gcp" || strings.ContainsRune(found[0].Description, '\x1b') {
			t.Errorf("got %q", found)
		}
		found, err = Search("")
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 5 || found[0].Name != "aws" || found[4].Name != "loop" {
			t.Errorf("got %q", found)
		}
	})
	t.Run("not JSON", func(t *testing.T) {
		index(t, "{")
		_, err := Search("a")
		mustFail(t, err, "index.json")
	})
	t.Run("a link", func(t *testing.T) {
		repo, url := newRepo(t, map[string]string{"real.json": `{"version": 1, "packs": []}`})
		if err := os.Symlink("real.json", filepath.Join(repo, "index.json")); err != nil {
			t.Skip(err)
		}
		commitAll(t, repo)
		write(t, config, fmt.Sprintf("[url %q]\n\tinsteadOf = https://github.com/dg-packs/index\n", url))
		_, err := Search("a")
		mustFail(t, err, "no file index.json")
	})
}

func TestDecodeIndex(t *testing.T) {
	tests := []struct {
		in, bad string
		n       int
	}{
		{in: `{"version": 1, "packs": [{"name": "a", "source": "b/c", "future": 1}]}`, n: 1},
		{in: `{"version": 1, "packs": [], "future": {}}`},
		{in: `{"version": 2, "packs": []}`, bad: "version 2"},
		{in: `{"packs": []}`, bad: "no version"},
		{in: `{"version": 1, "packs": [{"name": 1}]}`, bad: "index.json"},
		{in: `[]`, bad: "index.json"},
		{in: ``, bad: "index.json"},
	}
	for _, tt := range tests {
		entries, err := decodeIndex([]byte(tt.in))
		if tt.bad != "" {
			mustFail(t, err, tt.bad)
		} else if err != nil || len(entries) != tt.n {
			t.Errorf("%s: %v, %v", tt.in, entries, err)
		}
	}
}

func TestNoGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Add(newProject(t, nil), "example.com/a", "aws")
	mustFail(t, err, "git on the path")
}

func TestLicenses(t *testing.T) {
	hermetic(t)
	repo, url := newRepo(t, map[string]string{"spec.hcl": "", "LICENSE": "", "LICENSE-ICONS.txt": "", "NOTICE": "", "README.md": ""})
	run(t, repo, "tag", "v1.0.0")
	in := mustAdd(t, newProject(t, nil), url, "aws")
	want := []string{"packs/aws/LICENSE", "packs/aws/LICENSE-ICONS.txt", "packs/aws/NOTICE"}
	if !slices.Equal(in.Licenses, want) {
		t.Errorf("got %v", in.Licenses)
	}
}
