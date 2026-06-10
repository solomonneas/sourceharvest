package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/escoffier-labs/sourceharvest/internal/adapter"
)

func TestJSONLExportsAdapterRecords(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"jsonl", fixturePath("generic.fixture.jsonl"), "--source", "demo", "--collection", "demo:collection", "--out", "-", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("records = %d, want 2: %s", len(lines), stdout.String())
	}
	for _, line := range lines {
		rec, err := adapter.Parse([]byte(line))
		if err != nil {
			t.Fatalf("invalid adapter record: %v\n%s", err, line)
		}
		if rec.Source.Kind != "demo" || rec.Collection.ExternalID != "demo:collection" {
			t.Fatalf("unexpected record scope: %#v", rec)
		}
	}
	var summary Summary
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("invalid summary: %v\n%s", err, stderr.String())
	}
	if summary.Records != 2 || summary.Files != 1 {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func TestJSONLWarnsOnInvalidLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.jsonl")
	lines := strings.Join([]string{
		`{"id":"one","text":"first record"}`,
		`{"id":`,
		`{"id":"two","text":"second record"}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"jsonl", path, "--source", "demo", "--collection", "demo:collection", "--out", "-", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	if got := strings.Split(strings.TrimSpace(stdout.String()), "\n"); len(got) != 2 {
		t.Fatalf("records = %d, want 2: %s", len(got), stdout.String())
	}
	var summary Summary
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("invalid summary: %v\n%s", err, stderr.String())
	}
	if len(summary.Warnings) != 1 || !strings.Contains(summary.Warnings[0], "invalid JSON") {
		t.Fatalf("missing invalid JSON warning: %#v", summary.Warnings)
	}
}

func TestMarkdownExportsAdapterRecords(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"markdown", fixturePath("notes"), "--source", "notes", "--collection", "notes:local", "--out", "-", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("records = %d, want 1: %s", len(lines), stdout.String())
	}
	rec, err := adapter.Parse([]byte(lines[0]))
	if err != nil {
		t.Fatalf("invalid adapter record: %v\n%s", err, lines[0])
	}
	if rec.Source.Kind != "notes" || rec.Item.Kind != "note" || rec.Raw.Format != "text/markdown" {
		t.Fatalf("unexpected markdown record: %#v", rec)
	}
	if len(rec.Artifacts) != 1 || rec.Artifacts[0].Kind != "file" {
		t.Fatalf("markdown artifact missing: %#v", rec.Artifacts)
	}
	var summary Summary
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("invalid summary: %v\n%s", err, stderr.String())
	}
	if summary.Records != 1 || summary.Files != 1 {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func TestFileHTMLAndJSONExporters(t *testing.T) {
	cases := []struct {
		name string
		args []string
		kind string
	}{
		{"files", []string{"files", fixturePath("files"), "--source", "files", "--collection", "files:local", "--out", "-", "--json"}, "file"},
		{"html", []string{"html", fixturePath("html"), "--source", "html", "--collection", "html:local", "--out", "-", "--json"}, "page"},
		{"json", []string{"json", fixturePath("records.json"), "--source", "json", "--collection", "json:local", "--records-path", "records", "--out", "-", "--json"}, "message"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit %d stderr=%s", code, stderr.String())
			}
			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			if len(lines) != 1 {
				t.Fatalf("records = %d, want 1: %s", len(lines), stdout.String())
			}
			rec, err := adapter.Parse([]byte(lines[0]))
			if err != nil {
				t.Fatalf("invalid adapter record: %v\n%s", err, lines[0])
			}
			if rec.Item.Kind != tc.kind {
				t.Fatalf("kind = %q, want %q: %#v", rec.Item.Kind, tc.kind, rec)
			}
			if strings.Contains(rec.Item.Text, "<script>") {
				t.Fatalf("html was not stripped: %q", rec.Item.Text)
			}
		})
	}
}

func TestGitLogExporter(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "demo.invalid")
	runGit(t, dir, "config", "user.name", "Demo User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("adapter contract git evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "docs: add adapter evidence")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gitlog", dir, "--source", "gitlog", "--collection", "repo:test", "--out", "-", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("records = %d, want 1: %s", len(lines), stdout.String())
	}
	rec, err := adapter.Parse([]byte(lines[0]))
	if err != nil {
		t.Fatalf("invalid adapter record: %v\n%s", err, lines[0])
	}
	if rec.Item.Kind != "event" || !strings.Contains(rec.Item.Text, "adapter evidence") {
		t.Fatalf("unexpected gitlog record: %#v", rec)
	}
}

func TestExportFilesArePrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	outPath := filepath.Join(dir, "notes.adapter.jsonl")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"markdown", fixturePath("notes"), "--source", "notes", "--collection", "notes:local", "--out", outPath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("output mode = %o, want 0600", got)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("output dir mode = %o, want 0700", got)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatalf("expected summary JSON on stdout")
	}
}

func TestFailedExportDoesNotReplaceExistingOutput(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "records.adapter.jsonl")
	original := []byte("keep existing adapter export\n")
	if err := os.WriteFile(outPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"json", fixturePath("records.json"), "--source", "json", "--collection", "json:local", "--records-path", "missing", "--out", outPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected failure")
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("existing output was replaced: %q", got)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".records.adapter.jsonl.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary outputs left behind: %v", matches)
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), Version) {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func fixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// gitCurrentBranch returns the name of the currently checked-out branch, which
// varies (main vs master) with the host git's init.defaultBranch setting.
func gitCurrentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}
