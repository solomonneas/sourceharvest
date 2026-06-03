package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/solomonneas/sourceharvest/internal/adapter"
)

const Version = "0.1.0"

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printHelp(stdout)
		return 0
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "sourceharvest %s\n", Version)
		return 0
	case "jsonl":
		if err := runJSONL(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(stderr, "error: unknown command", args[0])
		return 1
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "sourceharvest exports local source-system records to logspine.adapter.v1 JSONL.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sourceharvest jsonl <path-or-dir> --source KIND --collection ID --out <file|-> [--collection-kind KIND] [--limit N] [--json]")
	fmt.Fprintln(w, "  sourceharvest version")
}

func runJSONL(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("jsonl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceKind := fs.String("source", "", "source kind")
	collectionID := fs.String("collection", "", "collection external ID")
	collectionKind := fs.String("collection-kind", "source_collection", "collection kind")
	outPath := fs.String("out", "-", "output file or - for stdout")
	limit := fs.Int("limit", 0, "maximum records to emit")
	jsonSummary := fs.Bool("json", false, "write summary JSON after export")
	path, flagArgs, err := splitPathAndFlags(args)
	if err != nil {
		return err
	}
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if path == "" || len(fs.Args()) != 0 {
		return errors.New("usage: sourceharvest jsonl <path-or-dir> --source KIND --collection ID --out <file|->")
	}
	if strings.TrimSpace(*sourceKind) == "" || strings.TrimSpace(*collectionID) == "" {
		return errors.New("--source and --collection are required")
	}
	var out io.Writer = stdout
	var file *os.File
	if *outPath != "-" {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil && filepath.Dir(*outPath) != "." {
			return err
		}
		f, err := os.Create(*outPath)
		if err != nil {
			return err
		}
		file = f
		out = f
	}
	result, err := exportJSONL(path, *sourceKind, *collectionID, *collectionKind, *limit, out)
	if file != nil {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return err
	}
	if *jsonSummary {
		target := stdout
		if *outPath == "-" {
			target = stderr
		}
		return writeJSON(target, result)
	}
	return nil
}

func splitPathAndFlags(args []string) (string, []string, error) {
	var path string
	var flags []string
	valueFlags := map[string]bool{"-source": true, "--source": true, "-collection": true, "--collection": true, "-collection-kind": true, "--collection-kind": true, "-out": true, "--out": true, "-limit": true, "--limit": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if strings.Contains(arg, "=") {
				continue
			}
			if valueFlags[arg] {
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("missing value for %s", arg)
				}
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		if path != "" {
			return "", nil, fmt.Errorf("unexpected argument %q", arg)
		}
		path = arg
	}
	return path, flags, nil
}

type Summary struct {
	Source      string   `json:"source"`
	Path        string   `json:"path"`
	Records     int      `json:"records"`
	Files       int      `json:"files"`
	Warnings    []string `json:"warnings"`
	GeneratedAt string   `json:"generated_at"`
}

func exportJSONL(root, sourceKind, collectionID, collectionKind string, limit int, w io.Writer) (Summary, error) {
	files, err := listJSONL(root)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		Source:      sourceKind,
		Path:        root,
		Files:       len(files),
		Warnings:    []string{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, file := range files {
		if limit > 0 && summary.Records >= limit {
			break
		}
		if err := scanFile(file, func(ordinal int64, line []byte, obj map[string]any) error {
			if limit > 0 && summary.Records >= limit {
				return nil
			}
			rec, warning := normalize(file, ordinal, line, obj, sourceKind, collectionID, collectionKind)
			if warning != "" {
				summary.Warnings = append(summary.Warnings, warning)
				return nil
			}
			if err := writeRecord(w, rec); err != nil {
				return err
			}
			summary.Records++
			return nil
		}); err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", file, err))
		}
	}
	return summary, nil
}

func normalize(path string, ordinal int64, line []byte, obj map[string]any, sourceKind, collectionID, collectionKind string) (adapter.Record, string) {
	text := firstString(obj, "text", "content", "message", "body", "title", "summary")
	if text == "" {
		return adapter.Record{}, fmt.Sprintf("%s:%d: no searchable text", path, ordinal)
	}
	externalID := firstString(obj, "id", "external_id", "url")
	if externalID == "" {
		externalID = sourceKind + ":" + stableID(path, fmt.Sprint(ordinal), text)
	} else if !strings.Contains(externalID, ":") {
		externalID = sourceKind + ":" + externalID
	}
	createdAt := firstString(obj, "created_at", "timestamp", "time", "date")
	actorName := firstString(obj, "author", "actor", "user", "username", "name")
	actorType := "system"
	if actorName != "" {
		actorType = "human"
	}
	ordinalCopy := ordinal
	rec := adapter.Record{
		Schema: adapter.SchemaV1,
		Source: adapter.Source{Kind: sourceKind, Name: sourceKind},
		Collection: adapter.Collection{
			ExternalID: collectionID,
			Kind:       collectionKind,
			Name:       collectionID,
			Metadata:   metadata(map[string]any{"source": sourceKind}),
		},
		Item: adapter.Item{
			ExternalID: externalID,
			Kind:       kindFrom(obj),
			CreatedAt:  createdAt,
			Text:       text,
			Tags:       []string{sourceKind},
			Metadata:   metadata(map[string]any{"source": sourceKind, "file_path": path, "ordinal": ordinal}),
		},
		Actor: &adapter.Actor{
			ExternalID: sourceKind + ":" + actorType + ":" + stableID(actorName),
			Type:       actorType,
			Name:       actorName,
		},
		Artifacts: []adapter.Artifact{},
		Links:     []adapter.Link{},
		Relations: []adapter.Relation{},
		Raw: adapter.RawRef{
			Format:  "json",
			Hash:    "sha256:" + hashBytes(line),
			Path:    path,
			Ordinal: &ordinalCopy,
		},
	}
	if url := firstString(obj, "url", "link", "uri"); url != "" {
		rec.Links = append(rec.Links, adapter.Link{URL: url, Text: firstString(obj, "title")})
	}
	if pathValue := firstString(obj, "path", "file_path"); pathValue != "" {
		rec.Artifacts = append(rec.Artifacts, adapter.Artifact{
			ExternalID: stableID(externalID, pathValue),
			Kind:       "file",
			Path:       pathValue,
			Hash:       "sha256:" + hashBytes([]byte(pathValue)),
		})
	}
	return rec, ""
}

func kindFrom(obj map[string]any) string {
	if kind := firstString(obj, "kind", "type"); kind != "" {
		return kind
	}
	return "message"
}

func listJSONL(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	var files []string
	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(root), ".jsonl") {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a JSONL file", root)
	}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == "backup" || name == "backups" || name == "deleted" {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(filepath.Base(path))
		if strings.HasSuffix(name, ".jsonl") && !strings.Contains(name, ".bak") && !strings.Contains(name, "backup") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func scanFile(path string, each func(ordinal int64, line []byte, obj map[string]any) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var ordinal int64
	for scanner.Scan() {
		ordinal++
		line := append([]byte(nil), scanner.Bytes()...)
		if strings.TrimSpace(string(line)) == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if err := each(ordinal, line, obj); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func writeRecord(w io.Writer, rec adapter.Record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func metadata(value map[string]any) json.RawMessage {
	b, _ := json.Marshal(value)
	return b
}

func stableID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = io.WriteString(h, part)
		_, _ = io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
