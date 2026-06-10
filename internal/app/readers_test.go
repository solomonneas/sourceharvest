package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/escoffier-labs/sourceharvest/internal/adapter"
)

// runReader executes a reader command and returns the parsed adapter records,
// the decoded summary, and the exit code. It fails the test on a non-zero exit
// or on any emitted line that is not a valid miseledger.adapter.v1 record.
func runReader(t *testing.T, args ...string) ([]adapter.Record, Summary) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	var recs []adapter.Record
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rec, err := adapter.Parse([]byte(line))
		if err != nil {
			t.Fatalf("invalid adapter record: %v\n%s", err, line)
		}
		recs = append(recs, rec)
	}
	var summary Summary
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("invalid summary: %v\n%s", err, stderr.String())
	}
	return recs, summary
}

// assertValidRecords checks the invariants every emitted record must satisfy,
// independent of the reader that produced it.
func assertValidRecords(t *testing.T, recs []adapter.Record, sourceKind, collectionID string) {
	t.Helper()
	for _, rec := range recs {
		if err := rec.Validate(); err != nil {
			t.Fatalf("record failed validation: %v\n%#v", err, rec)
		}
		if rec.Schema != adapter.SchemaV1 {
			t.Fatalf("schema = %q, want %q", rec.Schema, adapter.SchemaV1)
		}
		if rec.Source.Kind != sourceKind {
			t.Fatalf("source.kind = %q, want %q", rec.Source.Kind, sourceKind)
		}
		if rec.Collection.ExternalID != collectionID {
			t.Fatalf("collection.external_id = %q, want %q", rec.Collection.ExternalID, collectionID)
		}
		if strings.TrimSpace(rec.Item.Text) == "" {
			t.Fatalf("emitted record with empty item.text: %#v", rec)
		}
	}
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- jsonl ---------------------------------------------------------------

func TestJSONLReaderHappyPathRecords(t *testing.T) {
	recs, summary := runReader(t, "jsonl", fixturePath("generic.fixture.jsonl"),
		"--source", "demo", "--collection", "demo:collection", "--out", "-", "--json")
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	assertValidRecords(t, recs, "demo", "demo:collection")
	if summary.Records != 2 || summary.Files != 1 {
		t.Fatalf("bad summary: %#v", summary)
	}
	// The first fixture line carries a url, which becomes a link.
	if len(recs[0].Links) != 1 || recs[0].Links[0].URL == "" {
		t.Fatalf("expected url link on first record: %#v", recs[0].Links)
	}
	if recs[0].Raw.Format != "json" || recs[0].Raw.Ordinal == nil {
		t.Fatalf("jsonl raw ref incomplete: %#v", recs[0].Raw)
	}
}

func TestJSONLReaderMalformedAndEmptyLines(t *testing.T) {
	path := writeTemp(t, "records.jsonl", []byte(strings.Join([]string{
		`{"id":"one","text":"valid record"}`,
		``,
		`{"id":"broken",`,
		`   `,
		`{"id":"two","text":"another valid record"}`,
		`{"id":"no-text-field"}`,
	}, "\n")))
	recs, summary := runReader(t, "jsonl", path,
		"--source", "demo", "--collection", "demo:collection", "--out", "-", "--json")
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2: %#v", len(recs), recs)
	}
	assertValidRecords(t, recs, "demo", "demo:collection")
	var sawInvalidJSON, sawNoText bool
	for _, w := range summary.Warnings {
		if strings.Contains(w, "invalid JSON") {
			sawInvalidJSON = true
		}
		if strings.Contains(w, "no searchable text") {
			sawNoText = true
		}
	}
	if !sawInvalidJSON || !sawNoText {
		t.Fatalf("expected invalid-JSON and no-text warnings: %#v", summary.Warnings)
	}
}

// --- json ----------------------------------------------------------------

func TestJSONReaderRecordsPath(t *testing.T) {
	recs, summary := runReader(t, "json", fixturePath("records.json"),
		"--source", "json", "--collection", "json:local", "--records-path", "records", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "json", "json:local")
	if recs[0].Item.Kind != "message" {
		t.Fatalf("item.kind = %q, want message", recs[0].Item.Kind)
	}
	if summary.Records != 1 {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func TestJSONReaderMalformedJSONFails(t *testing.T) {
	path := writeTemp(t, "broken.json", []byte(`{"records": [ {"id": `))
	var stdout, stderr bytes.Buffer
	code := Run([]string{"json", path, "--source", "json", "--collection", "json:local",
		"--records-path", "records", "--out", "-"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected failure on malformed JSON, got success: %s", stdout.String())
	}
}

func TestJSONReaderNonObjectRecordsWarn(t *testing.T) {
	path := writeTemp(t, "mixed.json", []byte(`{"records":[{"id":"ok","text":"valid"},"not-an-object",42]}`))
	recs, summary := runReader(t, "json", path,
		"--source", "json", "--collection", "json:local", "--records-path", "records", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "json", "json:local")
	var sawNotObject int
	for _, w := range summary.Warnings {
		if strings.Contains(w, "not an object") {
			sawNotObject++
		}
	}
	if sawNotObject != 2 {
		t.Fatalf("expected 2 non-object warnings: %#v", summary.Warnings)
	}
}

// --- markdown ------------------------------------------------------------

func TestMarkdownReaderHappyPath(t *testing.T) {
	recs, summary := runReader(t, "markdown", fixturePath("notes"),
		"--source", "notes", "--collection", "notes:local", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "notes", "notes:local")
	rec := recs[0]
	if rec.Item.Kind != "note" || rec.Raw.Format != "text/markdown" {
		t.Fatalf("unexpected markdown record: %#v", rec)
	}
	if len(rec.Artifacts) != 1 || rec.Artifacts[0].MimeType != "text/markdown" {
		t.Fatalf("markdown artifact wrong: %#v", rec.Artifacts)
	}
	// Title is taken from the first heading.
	var meta map[string]any
	if err := json.Unmarshal(rec.Item.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["title"] != "Adapter Contract Notes" {
		t.Fatalf("title = %v, want heading text", meta["title"])
	}
	if summary.Records != 1 {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func TestMarkdownReaderParsesFrontMatter(t *testing.T) {
	dir := t.TempDir()
	doc := "---\n" +
		"title: Quarterly Notes\n" +
		"date: 2023-04-15\n" +
		"author: Jane Author\n" +
		"tags: [planning, ops]\n" +
		"status: draft\n" +
		"---\n" +
		"# Heading\n\nBody prose goes here.\n"
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, _ := runReader(t, "markdown", dir,
		"--source", "notes", "--collection", "notes:local", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "notes", "notes:local")
	rec := recs[0]

	// front matter is stripped from the body text.
	if strings.Contains(rec.Item.Text, "title:") || strings.Contains(rec.Item.Text, "---") {
		t.Fatalf("front matter leaked into text: %q", rec.Item.Text)
	}
	if !strings.Contains(rec.Item.Text, "Body prose goes here.") {
		t.Fatalf("body missing: %q", rec.Item.Text)
	}

	// title from front matter, not the heading.
	var meta map[string]any
	if err := json.Unmarshal(rec.Item.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["title"] != "Quarterly Notes" {
		t.Fatalf("title = %v, want front-matter title", meta["title"])
	}
	// unknown scalar keys survive in front_matter metadata.
	fm, ok := meta["front_matter"].(map[string]any)
	if !ok || fm["status"] != "draft" {
		t.Fatalf("front_matter metadata wrong: %v", meta["front_matter"])
	}

	// date drives created_at.
	if !strings.HasPrefix(rec.Item.CreatedAt, "2023-04-15") {
		t.Fatalf("created_at = %q, want front-matter date", rec.Item.CreatedAt)
	}

	// tags include front-matter tags alongside the defaults.
	tagset := map[string]bool{}
	for _, tag := range rec.Item.Tags {
		tagset[tag] = true
	}
	if !tagset["planning"] || !tagset["ops"] || !tagset["markdown"] {
		t.Fatalf("tags = %v, want planning+ops+markdown", rec.Item.Tags)
	}

	// author becomes a human actor.
	if rec.Actor == nil || rec.Actor.Type != "human" || rec.Actor.Name != "Jane Author" {
		t.Fatalf("actor = %#v, want human Jane Author", rec.Actor)
	}
}

func TestMarkdownReaderWithoutFrontMatterUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Plain Heading\n\nplain body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, _ := runReader(t, "markdown", dir,
		"--source", "notes", "--collection", "notes:local", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	rec := recs[0]
	var meta map[string]any
	if err := json.Unmarshal(rec.Item.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	// title falls back to the first heading.
	if meta["title"] != "Plain Heading" {
		t.Fatalf("title = %v, want heading", meta["title"])
	}
	if _, ok := meta["front_matter"]; ok {
		t.Fatalf("front_matter metadata should be absent: %v", meta["front_matter"])
	}
	// no author -> system actor (existing behavior preserved).
	if rec.Actor == nil || rec.Actor.Type != "system" {
		t.Fatalf("actor = %#v, want system actor", rec.Actor)
	}
}

func TestMarkdownReaderMalformedFrontMatterFallsBack(t *testing.T) {
	dir := t.TempDir()
	// opening fence with no closing fence: must not crash and must keep content.
	doc := "---\ntitle: Broken\nno closing fence here\n\n# Heading\n\nthe body\n"
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, summary := runReader(t, "markdown", dir,
		"--source", "notes", "--collection", "notes:local", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1 (summary=%#v)", len(recs), summary)
	}
	assertValidRecords(t, recs, "notes", "notes:local")
	rec := recs[0]
	// With no closing fence the whole document is the body (current behavior).
	if !strings.Contains(rec.Item.Text, "the body") || !strings.Contains(rec.Item.Text, "title: Broken") {
		t.Fatalf("malformed front matter should remain in body: %q", rec.Item.Text)
	}
	var meta map[string]any
	if err := json.Unmarshal(rec.Item.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if _, ok := meta["front_matter"]; ok {
		t.Fatalf("malformed front matter should not populate metadata: %v", meta["front_matter"])
	}
}

func TestMarkdownReaderEmptyFileWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.md"), []byte("   \n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.md"), []byte("# Title\n\nbody text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, summary := runReader(t, "markdown", dir,
		"--source", "notes", "--collection", "notes:local", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "notes", "notes:local")
	var sawEmpty bool
	for _, w := range summary.Warnings {
		if strings.Contains(w, "empty markdown file") {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Fatalf("expected empty markdown warning: %#v", summary.Warnings)
	}
}

// --- files ---------------------------------------------------------------

func TestFilesReaderHappyPath(t *testing.T) {
	recs, summary := runReader(t, "files", fixturePath("files"),
		"--source", "files", "--collection", "files:local", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "files", "files:local")
	rec := recs[0]
	if rec.Item.Kind != "file" {
		t.Fatalf("item.kind = %q, want file", rec.Item.Kind)
	}
	if len(rec.Artifacts) != 1 || rec.Artifacts[0].MimeType != "text/plain" {
		t.Fatalf("file artifact wrong: %#v", rec.Artifacts)
	}
	if summary.Records != 1 {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func TestFilesReaderEmptyAndBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blank.txt"), []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("usable evidence text"), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, summary := runReader(t, "files", dir,
		"--source", "files", "--collection", "files:local", "--glob", "*.txt", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "files", "files:local")
	var sawEmpty bool
	for _, w := range summary.Warnings {
		if strings.Contains(w, "empty text") {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Fatalf("expected empty-text warning: %#v", summary.Warnings)
	}
}

func TestFilesReaderGlobFiltersExtensions(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"keep.md":   "# kept markdown",
		"skip.json": `{"ignored":true}`,
		"keep.txt":  "kept text",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recs, _ := runReader(t, "files", dir,
		"--source", "files", "--collection", "files:local", "--glob", "*.md,*.txt", "--out", "-", "--json")
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2 (json should be excluded)", len(recs))
	}
}

// --- html ----------------------------------------------------------------

func TestHTMLReaderStripsMarkupAndKeepsTitle(t *testing.T) {
	recs, _ := runReader(t, "html", fixturePath(filepath.Join("html-realistic", "realistic.html")),
		"--source", "docs", "--collection", "docs:html", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "docs", "docs:html")
	rec := recs[0]
	if rec.Item.Kind != "page" {
		t.Fatalf("item.kind = %q, want page", rec.Item.Kind)
	}
	text := rec.Item.Text
	// script and style bodies must be stripped.
	if strings.Contains(text, "should not appear") || strings.Contains(text, "console.log") {
		t.Fatalf("script body leaked into text: %q", text)
	}
	if strings.Contains(text, "color:#333") || strings.Contains(text, "display:none") {
		t.Fatalf("style body leaked into text: %q", text)
	}
	if strings.Contains(text, "<") || strings.Contains(text, ">") {
		t.Fatalf("html tags leaked into text: %q", text)
	}
	// entities are decoded.
	if !strings.Contains(text, "12% over Q1") {
		t.Fatalf("entity not decoded: %q", text)
	}
	if !strings.Contains(text, "Adapter exports stayed local-only") {
		t.Fatalf("list text missing: %q", text)
	}
	// title comes from the <title> element, with entities decoded.
	var meta map[string]any
	if err := json.Unmarshal(rec.Item.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["title"] != "Quarterly Report & Notes" {
		t.Fatalf("title = %v, want decoded <title>", meta["title"])
	}
}

func TestHTMLReaderTitleFallsBackToFilename(t *testing.T) {
	recs, _ := runReader(t, "html", fixturePath(filepath.Join("html-realistic", "no-title.html")),
		"--source", "docs", "--collection", "docs:html", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "docs", "docs:html")
	var meta map[string]any
	if err := json.Unmarshal(recs[0].Item.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["title"] != "no-title" {
		t.Fatalf("title = %v, want filename fallback", meta["title"])
	}
}

func TestHTMLReaderEmptyFileWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blank.html"), []byte("<html><body>   </body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "real.html"), []byte("<html><body><p>page text</p></body></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, summary := runReader(t, "html", dir,
		"--source", "docs", "--collection", "docs:html", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	assertValidRecords(t, recs, "docs", "docs:html")
	var sawEmpty bool
	for _, w := range summary.Warnings {
		if strings.Contains(w, "empty text") {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Fatalf("expected empty-text warning: %#v", summary.Warnings)
	}
}

// --- gitlog --------------------------------------------------------------

func TestGitLogReaderRealisticHistory(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "author.invalid")
	runGit(t, dir, "config", "user.name", "Jane Maintainer")
	for i, msg := range []string{"feat: first commit", "fix: handle edge case with \"quotes\" and | pipe"} {
		name := filepath.Join(dir, "file"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(name, []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", msg)
	}
	recs, summary := runReader(t, "gitlog", dir,
		"--source", "gitlog", "--collection", "repo:test", "--out", "-", "--json")
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	assertValidRecords(t, recs, "gitlog", "repo:test")
	// git log is newest-first, so the fix commit comes first.
	first := recs[0]
	if first.Item.Kind != "event" {
		t.Fatalf("item.kind = %q, want event", first.Item.Kind)
	}
	if !strings.Contains(first.Item.Text, "handle edge case") {
		t.Fatalf("commit subject missing: %q", first.Item.Text)
	}
	if first.Actor == nil || first.Actor.Name != "Jane Maintainer" || first.Actor.Type != "human" {
		t.Fatalf("commit actor wrong: %#v", first.Actor)
	}
	if first.Raw.Ordinal == nil {
		t.Fatalf("gitlog raw ordinal missing: %#v", first.Raw)
	}
	// each commit carries a repo artifact (plus one file artifact per changed
	// file) and the commit hash in metadata.
	if first.Artifacts[0].Kind != "repo" {
		t.Fatalf("first artifact should be the repo: %#v", first.Artifacts)
	}
	var sawRepo bool
	for _, a := range first.Artifacts {
		if a.Kind == "repo" {
			sawRepo = true
		}
	}
	if !sawRepo {
		t.Fatalf("repo artifact missing: %#v", first.Artifacts)
	}
	if summary.Records != 2 || len(summary.Warnings) != 0 {
		t.Fatalf("bad summary: %#v", summary)
	}
}

func TestGitLogReaderCapturesBodyEmailAndFiles(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "jane@maintainer.invalid")
	runGit(t, dir, "config", "user.name", "Jane Maintainer")
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("beta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: add alpha and beta", "-m", "This is the body.\n\nWith a second paragraph and | pipe.")

	recs, summary := runReader(t, "gitlog", dir,
		"--source", "gitlog", "--collection", "repo:test", "--out", "-", "--json")
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	if summary.Records != 1 || len(summary.Warnings) != 0 {
		t.Fatalf("bad summary: %#v", summary)
	}
	assertValidRecords(t, recs, "gitlog", "repo:test")
	rec := recs[0]

	if !strings.Contains(rec.Item.Text, "feat: add alpha and beta") {
		t.Fatalf("subject missing from text: %q", rec.Item.Text)
	}
	if !strings.Contains(rec.Item.Text, "second paragraph and | pipe") {
		t.Fatalf("body missing from text: %q", rec.Item.Text)
	}

	// author email lands on the actor metadata.
	if rec.Actor == nil {
		t.Fatalf("actor missing")
	}
	var actorMeta map[string]any
	if err := json.Unmarshal(rec.Actor.Metadata, &actorMeta); err != nil {
		t.Fatalf("actor metadata: %v", err)
	}
	if actorMeta["email"] != "jane@maintainer.invalid" {
		t.Fatalf("author email = %v, want jane@maintainer.invalid", actorMeta["email"])
	}

	// item metadata carries the body and the changed-file list.
	var itemMeta map[string]any
	if err := json.Unmarshal(rec.Item.Metadata, &itemMeta); err != nil {
		t.Fatalf("item metadata: %v", err)
	}
	if body, _ := itemMeta["body"].(string); !strings.Contains(body, "second paragraph") {
		t.Fatalf("item metadata body wrong: %v", itemMeta["body"])
	}
	cf, ok := itemMeta["changed_files"].([]any)
	if !ok || len(cf) != 2 {
		t.Fatalf("changed_files = %v, want 2 entries", itemMeta["changed_files"])
	}

	// changed files also appear as "file" artifacts (plus the repo artifact).
	var fileArtifacts []adapter.Artifact
	var sawRepo bool
	paths := map[string]bool{}
	for _, a := range rec.Artifacts {
		switch a.Kind {
		case "repo":
			sawRepo = true
		case "file":
			fileArtifacts = append(fileArtifacts, a)
			paths[a.Path] = true
		}
	}
	if !sawRepo {
		t.Fatalf("repo artifact missing: %#v", rec.Artifacts)
	}
	if len(fileArtifacts) != 2 || !paths["alpha.txt"] || !paths["beta.txt"] {
		t.Fatalf("file artifacts wrong: %#v", fileArtifacts)
	}
}

func TestGitLogReaderHandlesEmptyBodyAndMerge(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "dev@example.invalid")
	runGit(t, dir, "config", "user.name", "Dev")
	// root commit with no body.
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "chore: base")
	defaultBranch := gitCurrentBranch(t, dir)
	// create a branch, diverge, and merge to produce a merge commit.
	runGit(t, dir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: feature work")
	runGit(t, dir, "checkout", defaultBranch)
	if err := os.WriteFile(filepath.Join(dir, "main.txt"), []byte("main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "feat: main work")
	runGit(t, dir, "merge", "--no-ff", "feature", "-m", "Merge feature")

	recs, summary := runReader(t, "gitlog", dir,
		"--source", "gitlog", "--collection", "repo:test", "--out", "-", "--json")
	if len(recs) == 0 {
		t.Fatalf("expected commits, got none")
	}
	assertValidRecords(t, recs, "gitlog", "repo:test")
	if len(summary.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", summary.Warnings)
	}
	// the base commit had no body; its text should equal its subject.
	var base *adapter.Record
	for i := range recs {
		if strings.Contains(recs[i].Item.Text, "chore: base") {
			base = &recs[i]
		}
	}
	if base == nil {
		t.Fatalf("base commit not found in %d records", len(recs))
	}
	if strings.TrimSpace(base.Item.Text) != "chore: base" {
		t.Fatalf("empty-body commit text = %q, want just the subject", base.Item.Text)
	}
}

func TestGitLogReaderRespectsLimit(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "author.invalid")
	runGit(t, dir, "config", "user.name", "Jane Maintainer")
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte{byte('0' + i)}, 0o600); err != nil {
			t.Fatal(err)
		}
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", "commit")
	}
	recs, _ := runReader(t, "gitlog", dir,
		"--source", "gitlog", "--collection", "repo:test", "--limit", "2", "--out", "-", "--json")
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2 with --limit 2", len(recs))
	}
}

func TestInvalidLimitSurfacesError(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "dev@example.invalid")
	runGit(t, dir, "config", "user.name", "Dev")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"gitlog", dir, "--source", "gitlog", "--collection", "repo:test",
		"--limit", "not-a-number", "--out", "-"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("invalid --limit should fail, got success: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "limit") {
		t.Fatalf("error should mention the limit flag, got: %s", stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("invalid --limit should emit no records, got: %s", stdout.String())
	}
}

func TestGitLogReaderEmptyRepoIsGraceful(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "author.invalid")
	runGit(t, dir, "config", "user.name", "Jane Maintainer")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gitlog", dir, "--source", "gitlog", "--collection", "repo:test", "--out", "-", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("empty repo should not be a hard error: exit %d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("empty repo should emit no records, got: %s", stdout.String())
	}
	var summary Summary
	if err := json.Unmarshal(stderr.Bytes(), &summary); err != nil {
		t.Fatalf("invalid summary: %v\n%s", err, stderr.String())
	}
	if summary.Records != 0 {
		t.Fatalf("records = %d, want 0", summary.Records)
	}
	if len(summary.Warnings) != 1 || !strings.Contains(summary.Warnings[0], "no commits yet") {
		t.Fatalf("expected no-commits warning: %#v", summary.Warnings)
	}
}

func TestGitLogReaderNonRepoFails(t *testing.T) {
	dir := t.TempDir() // a real directory that is not a git repo
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gitlog", dir, "--source", "gitlog", "--collection", "repo:test", "--out", "-"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("non-repo path should fail, got success")
	}
	if !strings.Contains(stderr.String(), "not a git repository") {
		t.Fatalf("expected git error surfaced, got: %s", stderr.String())
	}
}
