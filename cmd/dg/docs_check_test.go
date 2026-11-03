package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/nvcnvn/dg/internal/ops"
)

// blobURL is where GitHub shows a file of the repository; the documentation links source files
// there, since the site would serve them as downloads.
const blobURL = "https://github.com/nvcnvn/dg/blob/main/"

var (
	fenceRE   = regexp.MustCompile("(?ms)^```.*?^```")
	codeRE    = regexp.MustCompile("`[^`\n]*`")
	linkRE    = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	headingRE = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*#*$`)
)

// TestDocLinks checks that every relative link and image of the documentation names a file the
// site publishes, every link to GitHub a file of the repository, and every anchor a heading of
// its page, so the links work on GitHub and on the site alike; and that no published page holds
// Liquid tags, which Jekyll would expand.
func TestDocLinks(t *testing.T) {
	excluded := pagesExcluded(t)
	for _, file := range docFiles(t) {
		data, err := os.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !isExcluded(file, excluded) && (strings.Contains(text, "{{") || strings.Contains(text, "{%")) {
			t.Errorf("%s holds {{ or {%%, which Jekyll would expand as Liquid", file)
		}
		prose := codeRE.ReplaceAllString(fenceRE.ReplaceAllString(text, ""), "")
		for _, m := range linkRE.FindAllStringSubmatch(prose, -1) {
			target := m[1]
			if rest, ok := strings.CutPrefix(target, blobURL); ok {
				p, _, _ := strings.Cut(rest, "#")
				if info, err := os.Stat(filepath.Join(repoRoot, p)); err != nil || info.IsDir() {
					t.Errorf("%s: link %s names no file of the repository", file, target)
				}
				continue
			}
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			p, anchor, _ := strings.Cut(target, "#")
			p, err := url.PathUnescape(p)
			if err != nil {
				t.Errorf("%s: link %s: %v", file, target, err)
				continue
			}
			dest := file
			if p != "" {
				dest = path.Join(path.Dir(file), p)
			}
			info, err := os.Stat(filepath.Join(repoRoot, dest))
			switch {
			case err != nil || strings.HasPrefix(dest, "../"):
				t.Errorf("%s: link %s names no file of the repository", file, target)
				continue
			case info.IsDir():
				t.Errorf("%s: link %s names a directory, which the site does not list; name a file", file, target)
				continue
			case isExcluded(dest, excluded):
				t.Errorf("%s: link %s names a file the site leaves out; link %s%s instead", file, target, blobURL, dest)
			case !strings.HasSuffix(dest, ".md") && !strings.HasSuffix(dest, ".svg"):
				t.Errorf("%s: link %s names a file the site would serve as a download; link %s%s instead", file, target, blobURL, dest)
			}
			if anchor != "" && strings.HasSuffix(dest, ".md") && !slices.Contains(anchors(t, dest), anchor) {
				t.Errorf("%s: link %s names no heading of %s", file, target, dest)
			}
		}
	}
}

// TestDocSite checks what GitHub Pages publishes from the repository: README.md, the pages of
// docs/ and the pictures in testdata/export, and nothing else, such as source code or test data.
func TestDocSite(t *testing.T) {
	out, err := exec.Command("git", "-C", repoRoot, "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		t.Skip("needs git to list the repository's files:", err)
	}
	excluded := pagesExcluded(t)
	for _, f := range strings.Fields(string(out)) {
		hidden := slices.ContainsFunc(strings.Split(f, "/"), func(seg string) bool {
			return strings.HasPrefix(seg, ".") || strings.HasPrefix(seg, "_")
		})
		if hidden || isExcluded(f, excluded) {
			continue
		}
		page := f == "README.md" || strings.HasPrefix(f, "docs/") && strings.HasSuffix(f, ".md")
		picture := path.Dir(f) == "testdata/export" && strings.HasSuffix(f, ".svg")
		if !page && !picture {
			t.Errorf("the site publishes %s; add it to the exclude list of _config.yml", f)
		}
	}
}

// pagesExcluded returns the patterns of the exclude list of _config.yml.
func pagesExcluded(t *testing.T) []string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	_, list, ok := strings.Cut(string(data), "\nexclude:\n")
	if !ok {
		t.Fatal("_config.yml has no exclude list")
	}
	var patterns []string
	for _, line := range strings.Split(list, "\n") {
		item, ok := strings.CutPrefix(line, "  - ")
		if !ok {
			break
		}
		patterns = append(patterns, strings.Trim(item, `"`))
	}
	return patterns
}

// isExcluded reports whether Jekyll leaves p out for an exclude pattern: one that matches it as a
// glob, or that it starts with.
func isExcluded(p string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(e string) bool {
		ok, _ := path.Match(e, p)
		return ok || strings.HasPrefix(p, e)
	})
}

// anchors returns the anchors of the headings of a Markdown file, a repeated one numbered.
func anchors(t *testing.T, file string) []string {
	data, err := os.ReadFile(filepath.Join(repoRoot, file))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	count := map[string]int{}
	for _, m := range headingRE.FindAllStringSubmatch(fenceRE.ReplaceAllString(string(data), ""), -1) {
		slug := slug(m[1])
		if n := count[slug]; n > 0 {
			out = append(out, fmt.Sprintf("%s-%d", slug, n))
		} else {
			out = append(out, slug)
		}
		count[slug]++
	}
	return out
}

var dgCommandRE = regexp.MustCompile(`\bdg (check|fmt|export|serve|vocabulary|op|mcp|lsp)\b([^\n|#]*)`)

// TestDocCommands checks that every dg command the documentation's code shows names an
// operation and flags that exist.
func TestDocCommands(t *testing.T) {
	flagsOf := map[string][]string{}
	known := func(args ...string) []string {
		key := strings.Join(args, " ")
		if _, ok := flagsOf[key]; !ok {
			names := []string{"help", "h"}
			for _, f := range helpFlags(help(t, append(args, "--help")...)) {
				names = append(names, f.name)
			}
			flagsOf[key] = names
		}
		return flagsOf[key]
	}
	for _, file := range docFiles(t) {
		data, err := os.ReadFile(filepath.Join(repoRoot, file))
		if err != nil {
			t.Fatal(err)
		}
		code := strings.Join(append(fenceRE.FindAllString(string(data), -1), codeRE.FindAllString(string(data), -1)...), "\n")
		code = strings.ReplaceAll(code, "`", "\n")
		for _, m := range dgCommandRE.FindAllStringSubmatch(code, -1) {
			args := strings.Fields(strings.NewReplacer("[", " ", "]", " ").Replace(m[2]))
			valid := known(m[1])
			if m[1] == "op" {
				valid = []string{"help", "h", "C", "json"}
			}
			if m[1] == "op" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
				name := args[0]
				args = args[1:]
				_, ok := ops.LookupDef(name)
				switch {
				case strings.HasPrefix(name, "<"):
				case !ok:
					t.Errorf("%s: %q names no operation", file, m[0])
					continue
				default:
					valid = known("op", name)
				}
			}
			for _, a := range args {
				if strings.HasPrefix(a, "<") {
					break
				}
				name, isFlag := strings.CutPrefix(a, "-")
				if !isFlag || name == "" {
					continue
				}
				name, _, _ = strings.Cut(strings.TrimPrefix(name, "-"), "=")
				if !slices.Contains(valid, name) {
					t.Errorf("%s: %q: dg %s has no flag -%s", file, strings.TrimSpace(m[0]), m[1], name)
				}
			}
		}
	}
}

var (
	savedAsRE = regexp.MustCompile("as `([^`]+)`[^\n]*:\n*$")
	blockRE   = regexp.MustCompile("(?ms)^```(\\w*)\n(.*?)^```")
)

// TestGettingStarted runs the guide from "Write a model" on: it writes the files the guide says
// to save, runs its commands other than dg serve in a new directory, and compares the output of
// a block of commands with the text block that follows it.
func TestGettingStarted(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "getting-started.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, guide, ok := strings.Cut(string(data), "\n## Write a model\n")
	if !ok {
		t.Fatal("the guide has no section Write a model")
	}
	dir := t.TempDir()
	var output string
	last := 0
	for _, m := range blockRE.FindAllStringSubmatchIndex(guide, -1) {
		before, lang, body := guide[last:m[0]], guide[m[2]:m[3]], guide[m[4]:m[5]]
		last = m[1]
		switch lang {
		case "hcl":
			if name := savedAsRE.FindStringSubmatch(before); name != nil {
				if err := os.WriteFile(filepath.Join(dir, name[1]), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		case "text":
			if strings.TrimSpace(before) == "" && output != body {
				t.Errorf("the guide shows the output\n%s\nbut the commands print\n%s", body, output)
			}
		case "sh":
			output = ""
			for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
				output += runGuideLine(t, &dir, line)
			}
		}
	}
}

// runGuideLine runs a command of the guide in dir and returns what dg printed.
func runGuideLine(t *testing.T, dir *string, line string) string {
	args := shellWords(t, line)
	switch {
	case args[0] == "mkdir" && len(args) == 2:
		if err := os.Mkdir(filepath.Join(*dir, args[1]), 0o755); err != nil {
			t.Fatal(err)
		}
	case args[0] == "cd" && len(args) == 2:
		*dir = filepath.Join(*dir, args[1])
	case args[0] == "echo" && len(args) == 4 && args[2] == ">":
		if err := os.WriteFile(filepath.Join(*dir, args[3]), []byte(args[1]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	case args[0] == "dg" && args[1] == "serve":
	case args[0] == "dg":
		if args[1] == "export" && testing.Short() {
			return ""
		}
		t.Chdir(*dir)
		var out bytes.Buffer
		if code := run(args[1:], &out, &out); code != 0 {
			t.Errorf("%s: exit code %d: %s", line, code, out.String())
		}
		return out.String()
	default:
		t.Errorf("the guide runs %q, which TestGettingStarted cannot run", line)
	}
	return ""
}

// shellWords splits a command line into words as a shell does, for the quoting the guide uses.
func shellWords(t *testing.T, line string) []string {
	var words []string
	var word strings.Builder
	var quote rune
	inWord := false
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		t.Fatalf("unterminated quote in %q", line)
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

// slug is the anchor GitHub and kramdown's GFM parser make of a heading: lower case, without
// punctuation, spaces as hyphens.
func slug(heading string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ' ':
			return '-'
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			return unicode.ToLower(r)
		}
		return -1
	}, heading)
}
