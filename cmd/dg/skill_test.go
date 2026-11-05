package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nvcnvn/dg/internal/mcp"
	"github.com/nvcnvn/dg/internal/ops"
)

const skillFile = repoRoot + "/skills/dg/SKILL.md"

var (
	skillNameRE   = regexp.MustCompile(`(?m)^name: (\S+)$`)
	skillDescRE   = regexp.MustCompile(`(?m)^description: (.+)$`)
	skillBlockRE  = regexp.MustCompile("(?ms)^```(hcl|json|sh)\n(.*?)^```")
	skillOpNameRE = regexp.MustCompile("`([a-z]+(?:_[a-z]+)+)`")
)

// TestSkillFrontMatter checks what the Agent Skills format requires: a name that matches the
// folder, and a description of at most 1024 characters.
func TestSkillFrontMatter(t *testing.T) {
	text := readSkill(t)
	front, _, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	if !ok {
		t.Fatal("no front matter")
	}
	name := skillNameRE.FindStringSubmatch(front)
	if name == nil || name[1] != filepath.Base(filepath.Dir(skillFile)) {
		t.Errorf("name = %v, want the folder's name", name)
	}
	desc := skillDescRE.FindStringSubmatch(front)
	if desc == nil || len(desc[1]) > 1024 {
		t.Errorf("description missing or over 1024 characters: %v", desc)
	}
}

// TestSkillURL checks that the URL dg gives agents names the skill, and that the README's top
// gives the same one.
func TestSkillURL(t *testing.T) {
	rel, ok := strings.CutPrefix(mcp.SkillURL, "https://raw.githubusercontent.com/nvcnvn/dg/main/")
	if !ok {
		t.Fatalf("SkillURL = %s, not a raw file of the repository", mcp.SkillURL)
	}
	if filepath.Join(repoRoot, rel) != filepath.Clean(skillFile) {
		t.Errorf("SkillURL names %s, not %s", rel, skillFile)
	}
	readme, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), mcp.SkillURL) {
		t.Error("the README does not give SkillURL")
	}
}

// TestSkillOperations checks that every operation the skill names exists; ordered_in, written
// the same way, is a field of the JSON export.
func TestSkillOperations(t *testing.T) {
	names := make([]string, 0, len(ops.Defs))
	for _, d := range ops.Defs {
		names = append(names, d.Name)
	}
	for _, m := range skillOpNameRE.FindAllStringSubmatch(readSkill(t), -1) {
		if m[1] != "ordered_in" && !slices.Contains(names, m[1]) {
			t.Errorf("the skill names %s, which is not an operation", m[1])
		}
	}
}

// TestSkillExamples runs the skill's examples in order, as an agent would: its HCL blocks are the
// model, the style and the views of one project, its JSON block a batch, and its sh block
// operations on the result. Every step must leave a project dg check accepts.
func TestSkillExamples(t *testing.T) {
	blocks := map[string][]string{}
	for _, m := range skillBlockRE.FindAllStringSubmatch(readSkill(t), -1) {
		blocks[m[1]] = append(blocks[m[1]], m[2])
	}
	if len(blocks["hcl"]) != 3 || len(blocks["json"]) != 1 || len(blocks["sh"]) != 1 {
		t.Fatalf("got %d hcl, %d json and %d sh blocks, want 3, 1 and 1", len(blocks["hcl"]), len(blocks["json"]), len(blocks["sh"]))
	}
	dir := t.TempDir()
	for i, name := range []string{"model.hcl", "style.hcl", "views.hcl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(blocks["hcl"][i]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	batch := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(batch, []byte(blocks["json"][0]), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "check", "-C", dir)
	mustRun(t, "op", "batch", "-C", dir, "--file", batch)
	mustRun(t, "check", "-C", dir)
	for _, line := range strings.Split(strings.TrimSpace(blocks["sh"][0]), "\n") {
		args := strings.Fields(line)
		if len(args) < 3 || args[0] != "dg" || args[1] != "op" {
			t.Fatalf("not a dg op command: %q", line)
		}
		mustRun(t, slices.Concat(args[1:3], []string{"-C", dir}, args[3:])...)
		mustRun(t, "check", "-C", dir)
	}
}

func readSkill(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustRun(t *testing.T, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("dg %s: exit %d\n%s%s", strings.Join(args, " "), code, stdout.String(), stderr.String())
	}
}
