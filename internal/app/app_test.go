package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/solomonneas/sourceharvest/internal/adapter"
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
