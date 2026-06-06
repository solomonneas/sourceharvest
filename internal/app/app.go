package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	stdhtml "html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/solomonneas/sourceharvest/internal/adapter"
)

const Version = "0.1.1"

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
	case "markdown":
		if err := runMarkdown(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	case "files":
		if err := runFiles(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	case "html":
		if err := runHTML(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	case "gitlog":
		if err := runGitLog(args[1:], stdout, stderr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	case "json":
		if err := runJSON(args[1:], stdout, stderr); err != nil {
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
	fmt.Fprintln(w, "  sourceharvest markdown <path-or-dir> --source KIND --collection ID --out <file|-> [--collection-kind KIND] [--limit N] [--json]")
	fmt.Fprintln(w, "  sourceharvest files <path-or-dir> --source KIND --collection ID --out <file|-> [--glob PATTERNS] [--limit N] [--json]")
	fmt.Fprintln(w, "  sourceharvest html <path-or-dir> --source KIND --collection ID --out <file|-> [--limit N] [--json]")
	fmt.Fprintln(w, "  sourceharvest gitlog <repo> --source KIND --collection ID --out <file|-> [--limit N] [--json]")
	fmt.Fprintln(w, "  sourceharvest json <file> --source KIND --collection ID --records-path PATH --out <file|-> [--limit N] [--json]")
	fmt.Fprintln(w, "  sourceharvest version")
}

func runMarkdown(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("markdown", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceKind := fs.String("source", "", "source kind")
	collectionID := fs.String("collection", "", "collection external ID")
	collectionKind := fs.String("collection-kind", "notes", "collection kind")
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
		return errors.New("usage: sourceharvest markdown <path-or-dir> --source KIND --collection ID --out <file|->")
	}
	if strings.TrimSpace(*sourceKind) == "" || strings.TrimSpace(*collectionID) == "" {
		return errors.New("--source and --collection are required")
	}
	return withOutput(*outPath, stdout, stderr, *jsonSummary, func(w io.Writer) (Summary, error) {
		return exportMarkdown(path, *sourceKind, *collectionID, *collectionKind, *limit, w)
	})
}

func runFiles(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCommonExport("files", args, stderr, "files")
	if err != nil {
		return err
	}
	globs := opts.FlagSet.String("glob", "*.txt,*.md,*.markdown", "comma-separated filename globs")
	if err := opts.FlagSet.Parse(opts.FlagArgs); err != nil {
		return err
	}
	if err := opts.Validate("sourceharvest files <path-or-dir> --source KIND --collection ID --out <file|->"); err != nil {
		return err
	}
	return withOutput(opts.OutPath, stdout, stderr, opts.JSONSummary, func(w io.Writer) (Summary, error) {
		return exportFiles(opts.Path, opts.SourceKind, opts.CollectionID, opts.CollectionKind, *globs, opts.Limit, w)
	})
}

func runHTML(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCommonExport("html", args, stderr, "html_pages")
	if err != nil {
		return err
	}
	if err := opts.FlagSet.Parse(opts.FlagArgs); err != nil {
		return err
	}
	if err := opts.Validate("sourceharvest html <path-or-dir> --source KIND --collection ID --out <file|->"); err != nil {
		return err
	}
	return withOutput(opts.OutPath, stdout, stderr, opts.JSONSummary, func(w io.Writer) (Summary, error) {
		return exportHTML(opts.Path, opts.SourceKind, opts.CollectionID, opts.CollectionKind, opts.Limit, w)
	})
}

func runGitLog(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCommonExport("gitlog", args, stderr, "git_repository")
	if err != nil {
		return err
	}
	if err := opts.FlagSet.Parse(opts.FlagArgs); err != nil {
		return err
	}
	if err := opts.Validate("sourceharvest gitlog <repo> --source KIND --collection ID --out <file|->"); err != nil {
		return err
	}
	return withOutput(opts.OutPath, stdout, stderr, opts.JSONSummary, func(w io.Writer) (Summary, error) {
		return exportGitLog(opts.Path, opts.SourceKind, opts.CollectionID, opts.CollectionKind, opts.Limit, w)
	})
}

func runJSON(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCommonExport("json", args, stderr, "source_collection")
	if err != nil {
		return err
	}
	recordsPath := opts.FlagSet.String("records-path", "", "dot path to an array of records")
	if err := opts.FlagSet.Parse(opts.FlagArgs); err != nil {
		return err
	}
	if err := opts.Validate("sourceharvest json <file> --source KIND --collection ID --records-path PATH --out <file|->"); err != nil {
		return err
	}
	return withOutput(opts.OutPath, stdout, stderr, opts.JSONSummary, func(w io.Writer) (Summary, error) {
		return exportJSON(opts.Path, opts.SourceKind, opts.CollectionID, opts.CollectionKind, *recordsPath, opts.Limit, w)
	})
}

type commonExportOptions struct {
	Path           string
	FlagArgs       []string
	FlagSet        *flag.FlagSet
	SourceKind     string
	CollectionID   string
	CollectionKind string
	OutPath        string
	Limit          int
	JSONSummary    bool
}

func parseCommonExport(name string, args []string, stderr io.Writer, defaultCollectionKind string) (commonExportOptions, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceKind := fs.String("source", "", "source kind")
	collectionID := fs.String("collection", "", "collection external ID")
	collectionKind := fs.String("collection-kind", defaultCollectionKind, "collection kind")
	outPath := fs.String("out", "-", "output file or - for stdout")
	limit := fs.Int("limit", 0, "maximum records to emit")
	jsonSummary := fs.Bool("json", false, "write summary JSON after export")
	path, flagArgs, err := splitPathAndFlags(args)
	if err != nil {
		return commonExportOptions{}, err
	}
	return commonExportOptions{Path: path, FlagArgs: flagArgs, FlagSet: fs, SourceKind: *sourceKind, CollectionID: *collectionID, CollectionKind: *collectionKind, OutPath: *outPath, Limit: *limit, JSONSummary: *jsonSummary}, nil
}

func (o *commonExportOptions) Validate(usage string) error {
	o.SourceKind = strings.TrimSpace(o.FlagSet.Lookup("source").Value.String())
	o.CollectionID = strings.TrimSpace(o.FlagSet.Lookup("collection").Value.String())
	o.CollectionKind = strings.TrimSpace(o.FlagSet.Lookup("collection-kind").Value.String())
	o.OutPath = o.FlagSet.Lookup("out").Value.String()
	o.JSONSummary = o.FlagSet.Lookup("json").Value.String() == "true"
	if _, err := fmt.Sscan(o.FlagSet.Lookup("limit").Value.String(), &o.Limit); err != nil {
		o.Limit = 0
	}
	if o.Path == "" || len(o.FlagSet.Args()) != 0 {
		return errors.New("usage: " + usage)
	}
	if o.SourceKind == "" || o.CollectionID == "" {
		return errors.New("--source and --collection are required")
	}
	return nil
}

func withOutput(outPath string, stdout, stderr io.Writer, jsonSummary bool, fn func(io.Writer) (Summary, error)) error {
	var out io.Writer = stdout
	var file *atomicOutput
	if outPath != "-" {
		f, err := createAtomicOutput(outPath)
		if err != nil {
			return err
		}
		file = f
		out = f.File
	}
	result, err := fn(out)
	if file != nil {
		if err != nil {
			if abortErr := file.Abort(); abortErr != nil {
				return abortErr
			}
		} else if commitErr := file.Commit(); commitErr != nil {
			err = commitErr
		}
	}
	if err != nil {
		return err
	}
	if jsonSummary {
		target := stdout
		if outPath == "-" {
			target = stderr
		}
		return writeJSON(target, result)
	}
	return nil
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
	return withOutput(*outPath, stdout, stderr, *jsonSummary, func(w io.Writer) (Summary, error) {
		return exportJSONL(path, *sourceKind, *collectionID, *collectionKind, *limit, w)
	})
}

type atomicOutput struct {
	Path string
	Temp string
	File *os.File
}

func createAtomicOutput(path string) (*atomicOutput, error) {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return &atomicOutput{Path: path, Temp: file.Name(), File: file}, nil
}

func (o *atomicOutput) Commit() error {
	if o == nil || o.File == nil {
		return nil
	}
	if err := o.File.Close(); err != nil {
		_ = os.Remove(o.Temp)
		o.File = nil
		return err
	}
	o.File = nil
	if err := os.Rename(o.Temp, o.Path); err != nil {
		_ = os.Remove(o.Temp)
		return err
	}
	return nil
}

func (o *atomicOutput) Abort() error {
	if o == nil {
		return nil
	}
	var err error
	if o.File != nil {
		err = o.File.Close()
		o.File = nil
	}
	if removeErr := os.Remove(o.Temp); removeErr != nil && !os.IsNotExist(removeErr) && err == nil {
		err = removeErr
	}
	return err
}

func splitPathAndFlags(args []string) (string, []string, error) {
	var path string
	var flags []string
	valueFlags := map[string]bool{"-source": true, "--source": true, "-collection": true, "--collection": true, "-collection-kind": true, "--collection-kind": true, "-out": true, "--out": true, "-limit": true, "--limit": true, "-glob": true, "--glob": true, "-records-path": true, "--records-path": true}
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
		}, func(warning string) {
			summary.Warnings = append(summary.Warnings, warning)
		}); err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", file, err))
		}
	}
	return summary, nil
}

func exportMarkdown(root, sourceKind, collectionID, collectionKind string, limit int, w io.Writer) (Summary, error) {
	files, err := listMarkdown(root)
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
		b, err := os.ReadFile(file)
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", file, err))
			continue
		}
		text := strings.TrimSpace(string(b))
		if text == "" {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: empty markdown file", file))
			continue
		}
		info, err := os.Stat(file)
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", file, err))
			continue
		}
		hash := hashBytes(b)
		rec := markdownRecord(file, text, hash, info.ModTime().UTC().Format(time.RFC3339Nano), sourceKind, collectionID, collectionKind)
		if err := writeRecord(w, rec); err != nil {
			return summary, err
		}
		summary.Records++
	}
	return summary, nil
}

func exportFiles(root, sourceKind, collectionID, collectionKind, globs string, limit int, w io.Writer) (Summary, error) {
	files, err := listFiles(root, splitCSV(globs), false)
	if err != nil {
		return Summary{}, err
	}
	return exportTextFiles(root, files, sourceKind, collectionID, collectionKind, "file", "text/plain", limit, w, func(path, text string) string {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	})
}

func exportHTML(root, sourceKind, collectionID, collectionKind string, limit int, w io.Writer) (Summary, error) {
	files, err := listFiles(root, []string{"*.html", "*.htm"}, false)
	if err != nil {
		return Summary{}, err
	}
	return exportTextFiles(root, files, sourceKind, collectionID, collectionKind, "page", "text/html", limit, w, func(path, text string) string {
		title := firstHTMLTitle(text)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		return title
	})
}

func exportTextFiles(root string, files []string, sourceKind, collectionID, collectionKind, itemKind, mimeType string, limit int, w io.Writer, titleFn func(string, string) string) (Summary, error) {
	summary := Summary{Source: sourceKind, Path: root, Files: len(files), Warnings: []string{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for _, file := range files {
		if limit > 0 && summary.Records >= limit {
			break
		}
		b, err := os.ReadFile(file)
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", file, err))
			continue
		}
		text := strings.TrimSpace(string(b))
		if mimeType == "text/html" {
			text = htmlToText(text)
		}
		if text == "" {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: empty text", file))
			continue
		}
		info, err := os.Stat(file)
		if err != nil {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s: %s", file, err))
			continue
		}
		hash := hashBytes(b)
		title := titleFn(file, string(b))
		rec := fileRecord(file, text, title, hash, info.ModTime().UTC().Format(time.RFC3339Nano), sourceKind, collectionID, collectionKind, itemKind, mimeType)
		if err := writeRecord(w, rec); err != nil {
			return summary, err
		}
		summary.Records++
	}
	return summary, nil
}

func exportGitLog(repo, sourceKind, collectionID, collectionKind string, limit int, w io.Writer) (Summary, error) {
	if limit <= 0 {
		limit = 200
	}
	cmd := exec.Command("git", "-C", repo, "log", "--date=iso-strict", "--format=%H%x1f%aI%x1f%an%x1f%s", "-n", fmt.Sprint(limit))
	b, err := cmd.Output()
	if err != nil {
		return Summary{}, err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	summary := Summary{Source: sourceKind, Path: repo, Files: 1, Warnings: []string{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 4)
		if len(parts) != 4 {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("git log line %d: malformed", i+1))
			continue
		}
		rec := gitLogRecord(repo, parts[0], parts[1], parts[2], parts[3], sourceKind, collectionID, collectionKind, int64(i+1))
		if err := writeRecord(w, rec); err != nil {
			return summary, err
		}
		summary.Records++
	}
	return summary, nil
}

func exportJSON(path, sourceKind, collectionID, collectionKind, recordsPath string, limit int, w io.Writer) (Summary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return Summary{}, err
	}
	records, err := selectJSONRecords(root, recordsPath)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Source: sourceKind, Path: path, Files: 1, Warnings: []string{}, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for i, item := range records {
		if limit > 0 && summary.Records >= limit {
			break
		}
		obj, ok := item.(map[string]any)
		if !ok {
			summary.Warnings = append(summary.Warnings, fmt.Sprintf("%s:%d: record is not an object", path, i+1))
			continue
		}
		line, _ := json.Marshal(obj)
		rec, warning := normalize(path, int64(i+1), line, obj, sourceKind, collectionID, collectionKind)
		if warning != "" {
			summary.Warnings = append(summary.Warnings, warning)
			continue
		}
		if err := writeRecord(w, rec); err != nil {
			return summary, err
		}
		summary.Records++
	}
	return summary, nil
}

func fileRecord(path, text, title, hash, createdAt, sourceKind, collectionID, collectionKind, itemKind, mimeType string) adapter.Record {
	externalID := sourceKind + ":" + itemKind + ":" + stableID(path, hash)
	return adapter.Record{
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
			Kind:       itemKind,
			CreatedAt:  createdAt,
			Text:       text,
			Tags:       []string{sourceKind, itemKind},
			Metadata:   metadata(map[string]any{"source": sourceKind, "file_path": path, "title": title}),
		},
		Actor: &adapter.Actor{
			ExternalID: sourceKind + ":system:file",
			Type:       "system",
			Name:       "File",
		},
		Artifacts: []adapter.Artifact{{
			ExternalID: stableID(externalID, path),
			Kind:       "file",
			Path:       path,
			MimeType:   mimeType,
			Hash:       "sha256:" + hash,
			Metadata:   metadata(map[string]any{"title": title}),
		}},
		Links:     []adapter.Link{},
		Relations: []adapter.Relation{},
		Raw:       adapter.RawRef{Format: mimeType, Hash: "sha256:" + hash, Path: path},
	}
}

func gitLogRecord(repo, commit, createdAt, author, subject, sourceKind, collectionID, collectionKind string, ordinal int64) adapter.Record {
	line := []byte(commit + "\x1f" + createdAt + "\x1f" + author + "\x1f" + subject)
	externalID := sourceKind + ":commit:" + commit
	ordinalCopy := ordinal
	return adapter.Record{
		Schema: adapter.SchemaV1,
		Source: adapter.Source{Kind: sourceKind, Name: sourceKind},
		Collection: adapter.Collection{
			ExternalID: collectionID,
			Kind:       collectionKind,
			Name:       collectionID,
			Metadata:   metadata(map[string]any{"repo": repo}),
		},
		Item: adapter.Item{
			ExternalID: externalID,
			Kind:       "event",
			CreatedAt:  createdAt,
			Text:       subject,
			Tags:       []string{sourceKind, "gitlog"},
			Metadata:   metadata(map[string]any{"repo": repo, "commit": commit}),
		},
		Actor: &adapter.Actor{
			ExternalID: sourceKind + ":author:" + stableID(author),
			Type:       "human",
			Name:       author,
		},
		Artifacts: []adapter.Artifact{{
			ExternalID: stableID(externalID, repo),
			Kind:       "repo",
			Path:       repo,
			Metadata:   metadata(map[string]any{"commit": commit}),
		}},
		Links:     []adapter.Link{},
		Relations: []adapter.Relation{},
		Raw: adapter.RawRef{
			Format:  "text/git-log",
			Hash:    "sha256:" + hashBytes(line),
			Path:    repo,
			Ordinal: &ordinalCopy,
		},
	}
}

func markdownRecord(path, text, hash, createdAt, sourceKind, collectionID, collectionKind string) adapter.Record {
	title := markdownTitle(path, text)
	externalID := sourceKind + ":markdown:" + stableID(path, hash)
	return adapter.Record{
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
			Kind:       "note",
			CreatedAt:  createdAt,
			Text:       text,
			Tags:       []string{sourceKind, "markdown"},
			Metadata:   metadata(map[string]any{"source": sourceKind, "file_path": path, "title": title}),
		},
		Actor: &adapter.Actor{
			ExternalID: sourceKind + ":system:markdown",
			Type:       "system",
			Name:       "Markdown",
		},
		Artifacts: []adapter.Artifact{{
			ExternalID: stableID(externalID, path),
			Kind:       "file",
			Path:       path,
			MimeType:   "text/markdown",
			Hash:       "sha256:" + hash,
			Metadata:   metadata(map[string]any{"title": title}),
		}},
		Links:     []adapter.Link{},
		Relations: []adapter.Relation{},
		Raw: adapter.RawRef{
			Format: "text/markdown",
			Hash:   "sha256:" + hash,
			Path:   path,
		},
	}
}

func markdownTitle(path, text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
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

func listMarkdown(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	var files []string
	if !info.IsDir() {
		name := strings.ToLower(root)
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown") {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a Markdown file", root)
	}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == ".git" || name == "node_modules" || name == "backup" || name == "backups" || name == "deleted" {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(filepath.Base(path))
		if (strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".markdown")) && !strings.Contains(name, ".bak") && !strings.Contains(name, "backup") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func listFiles(root string, globs []string, includeHidden bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	var files []string
	if !info.IsDir() {
		if matchesAny(filepath.Base(root), globs) {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s does not match requested globs", root)
	}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		lower := strings.ToLower(name)
		if d.IsDir() {
			if lower == ".git" || lower == "node_modules" || lower == "backup" || lower == "backups" || lower == "deleted" {
				return filepath.SkipDir
			}
			if !includeHidden && strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !includeHidden && strings.HasPrefix(name, ".") {
			return nil
		}
		if matchesAny(name, globs) && !strings.Contains(lower, ".bak") && !strings.Contains(lower, "backup") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func splitCSV(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func matchesAny(name string, globs []string) bool {
	for _, glob := range globs {
		if ok, _ := filepath.Match(glob, name); ok {
			return true
		}
	}
	return false
}

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	tagRe         = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRe       = regexp.MustCompile(`[ \t\r\n]+`)
	titleRe       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

func htmlToText(raw string) string {
	raw = scriptStyleRe.ReplaceAllString(raw, " ")
	raw = tagRe.ReplaceAllString(raw, " ")
	raw = stdhtml.UnescapeString(raw)
	return strings.TrimSpace(spaceRe.ReplaceAllString(raw, " "))
}

func firstHTMLTitle(raw string) string {
	match := titleRe.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(stdhtml.UnescapeString(tagRe.ReplaceAllString(match[1], " ")))
}

func selectJSONRecords(root any, recordsPath string) ([]any, error) {
	cur := root
	if strings.TrimSpace(recordsPath) != "" {
		for _, part := range strings.Split(recordsPath, ".") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("records path %q does not resolve through an object", recordsPath)
			}
			cur = m[part]
		}
	}
	if arr, ok := cur.([]any); ok {
		return arr, nil
	}
	if m, ok := cur.(map[string]any); ok && recordsPath == "" {
		return []any{m}, nil
	}
	return nil, fmt.Errorf("records path %q did not resolve to an array", recordsPath)
}

func scanFile(path string, each func(ordinal int64, line []byte, obj map[string]any) error, warn func(string)) error {
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
			if warn != nil {
				warn(fmt.Sprintf("%s:%d: invalid JSON: %s", path, ordinal, err))
			}
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
