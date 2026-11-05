package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// release is what a fake GitHub release serves: an archive of a dg holding binary per target,
// and SHA256SUMS. corrupt lists the hash of other bytes; unlisted leaves the archives out of it.
type release struct {
	tag, binary       string
	corrupt, unlisted bool
}

// archive packs the binary as make dist-<target> does, as a zip on Windows.
func (r release) archive(t *testing.T, target string) (name string, data []byte) {
	t.Helper()
	dir := "dg-" + r.tag + "-" + target
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		z := zip.NewWriter(&buf)
		w, err := z.Create(dir + "/dg.exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(r.binary)); err != nil {
			t.Fatal(err)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		return dir + ".zip", buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: dir + "/dg", Mode: 0o755, Size: int64(len(r.binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(r.binary)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return dir + ".tar.gz", buf.Bytes()
}

// serveReleases serves releases as GitHub lays them out, the first as the latest. Both
// architectures exist because the script's shell may be an emulated one.
func serveReleases(t *testing.T, releases []release) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/"+releases[0].tag, http.StatusFound)
	})
	mux.HandleFunc("/tag/", func(http.ResponseWriter, *http.Request) {})
	for _, rel := range releases {
		var sums strings.Builder
		for _, arch := range []string{"amd64", "arm64"} {
			name, data := rel.archive(t, runtime.GOOS+"-"+arch)
			mux.HandleFunc("/download/"+rel.tag+"/"+name, func(w http.ResponseWriter, _ *http.Request) {
				w.Write(data)
			})
			sum := sha256.Sum256(data)
			if rel.corrupt {
				sum = sha256.Sum256(nil)
			}
			if !rel.unlisted {
				fmt.Fprintf(&sums, "%x  %s\n", sum, name)
			}
		}
		mux.HandleFunc("/download/"+rel.tag+"/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, sums.String())
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestInstallScript pipes install.sh to bash, as the documented curl command does, against a
// local server. The installed file comes from an archive the test made; only its bytes matter.
func TestInstallScript(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("needs bash:", err)
	}
	script, err := os.ReadFile(filepath.Join(repoRoot, "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	latest := release{tag: "v1.2.3", binary: "latest"}
	older := release{tag: "v1.2.2", binary: "older"}
	corrupt := release{tag: "v1.2.1", binary: "corrupt", corrupt: true}
	unlisted := release{tag: "v1.2.0", binary: "unlisted", unlisted: true}
	url := serveReleases(t, []release{latest, older, corrupt, unlisted})

	tests := []struct {
		name    string
		version string
		before  string
		want    string
		failure string
	}{
		{name: "latest", want: "latest"},
		{name: "a tag", version: "v1.2.2", want: "older"},
		{name: "a version without v", version: "1.2.2", want: "older"},
		{name: "replaces a copy", version: "v1.2.2", before: "old copy", want: "older"},
		{name: "wrong checksum", version: "v1.2.1", before: "old copy", want: "old copy", failure: "checksum mismatch"},
		{name: "not in SHA256SUMS", version: "v1.2.0", failure: "does not list"},
		{name: "no such release", version: "v9.9.9", failure: "no release v9.9.9"},
		{name: "not a version", version: "v1.2.3/../../x", failure: "not a release"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "dg")
			if runtime.GOOS == "windows" {
				exe += ".exe"
			}
			if tt.before != "" {
				if err := os.WriteFile(exe, []byte(tt.before), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.Command(bash)
			cmd.Stdin = bytes.NewReader(script)
			cmd.Env = append(os.Environ(),
				"DG_RELEASES_URL="+url,
				"DG_INSTALL_DIR="+filepath.ToSlash(dir),
				"DG_VERSION="+tt.version,
			)
			out, err := cmd.CombinedOutput()

			if tt.failure == "" && err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if tt.failure != "" && (err == nil || !strings.Contains(string(out), tt.failure)) {
				t.Fatalf("error %v, output\n%s\nwant a failure that says %q", err, out, tt.failure)
			}
			got, err := os.ReadFile(exe)
			if tt.want == "" {
				if !os.IsNotExist(err) {
					t.Errorf("%s exists after a failure: %v", exe, err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if string(got) != tt.want {
				t.Errorf("installed %q, want %q", got, tt.want)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if i := slices.IndexFunc(entries, func(e os.DirEntry) bool { return e.Name() != filepath.Base(exe) }); i >= 0 {
				t.Errorf("left %s behind", entries[i].Name())
			}
		})
	}
}
