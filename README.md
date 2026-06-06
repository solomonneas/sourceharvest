# SourceHarvest

SourceHarvest exports source-system records to `logspine.adapter.v1` JSONL.

It is the sibling tool to AgentTrail:

- [AgentTrail](https://github.com/solomonneas/agenttrail) handles local agent-session harnesses such as Codex, Claude, OpenClaw, OpenCode, and Hermes.
- SourceHarvest handles non-harness source systems such as crawler exports, notes, chat exports, issue exports, and future domain-specific harvesters.
- [Logspine](https://github.com/solomonneas/logspine) stores, dedupes, indexes, searches, relates, and emits evidence bundles.

SourceHarvest is not an archive.

## How It Works

```mermaid
flowchart TB
    SOURCEHARVEST["<b>SourceHarvest CLI</b><br/><i>local adapter layer</i>"]
    ADAPTER["<b>logspine.adapter.v1 JSONL</b><br/>one normalized object per line"]

    subgraph INPUTS [" local source inputs "]
        JSONL["<b>Generic JSONL</b><br/>already line-oriented records"]
        JSON["<b>Nested JSON</b><br/>records selected by path"]
        NOTES["<b>Markdown notes</b><br/>local note evidence"]
        FILES["<b>Text files</b><br/>docs · logs · exports"]
        HTML["<b>HTML exports</b><br/>local page snapshots"]
        GITLOG["<b>Git history</b><br/>local commit events"]
    end

    JSONL & JSON & NOTES & FILES & HTML & GITLOG --> SOURCEHARVEST

    subgraph PIPELINE [" normalize and emit "]
        READ["<b>Read local input</b><br/>file or directory scan"]
        SELECT["<b>Select reader</b><br/>jsonl · json · markdown · files · html · gitlog"]
        NORMALIZE["<b>Normalize record</b><br/>collection · item · actor · artifacts · raw"]
        FILTER["<b>Apply bounds</b><br/>limit · globs · records path"]
        EMIT["<b>Emit JSONL</b><br/>stdout or private output file"]
    end

    SOURCEHARVEST --> READ --> SELECT --> NORMALIZE --> FILTER --> EMIT
    EMIT == adapter records ==> ADAPTER

    SUMMARY["<b>JSON summary</b><br/>records · files · warnings · generated_at"]
    EMIT -->|optional JSON summary| SUMMARY

    BOUNDARY["<b>Boundary</b><br/>scanner commands stay local-only; exported text is untrusted evidence"]
    INPUTS -. local files only .-> BOUNDARY
    NORMALIZE -. treats text as data .-> BOUNDARY
    BOUNDARY -. constrains .-> PIPELINE

    classDef source fill:#eff6ff,stroke:#2563eb,color:#1e3a8a;
    classDef process fill:#ecfdf5,stroke:#059669,color:#064e3b;
    classDef output fill:#fff7ed,stroke:#ea580c,color:#7c2d12;
    classDef guard fill:#fee2e2,stroke:#ef4444,color:#7f1d1d;
    class SOURCEHARVEST,JSONL,JSON,NOTES,FILES,HTML,GITLOG source;
    class READ,SELECT,NORMALIZE,FILTER process;
    class EMIT,ADAPTER,SUMMARY output;
    class BOUNDARY guard;
```

Editable Excalidraw source: [docs/sourceharvest-flowcharts.excalidraw](docs/sourceharvest-flowcharts.excalidraw)

SourceHarvest follows the same path for each source:

1. Read a local file, directory, export, or source archive.
2. Select the command-specific reader for that input shape.
3. Normalize records into stable collections, items, actors, artifacts, links, relations, and raw references.
4. Apply `--limit` and source-specific filters.
5. Emit one `logspine.adapter.v1` JSON object per line.
6. Optionally emit JSON summaries with record counts, file counts, warnings, and generated timestamps.

## With Logspine And AgentTrail

```mermaid
flowchart TB
    SOURCEHARVEST["<b>SourceHarvest</b><br/><i>non-agent source adapters</i>"]
    AGENTTRAIL["<b>AgentTrail</b><br/><i>agent-session adapters</i>"]
    LOGSPINE["<b>Logspine</b><br/>durable evidence store"]

    subgraph CRAWLERS [" local crawler exports "]
        DISCRAWL["<b>discrawl</b><br/>Discord archives"]
        GITCRAWL["<b>gitcrawl</b><br/>GitHub issues and pull requests"]
        GRAINCRAWL["<b>graincrawl</b><br/>Granola notes and transcripts"]
        NOTCRAWL["<b>notcrawl</b><br/>Notion pages and databases"]
        SLACRAWL["<b>slacrawl</b><br/>Slack messages and threads"]
        TELECRAWL["<b>telecrawl</b><br/>Telegram Desktop archives"]
    end

    subgraph LOCAL [" local source files "]
        NOTES["<b>Notes</b><br/>markdown · text"]
        EXPORTS["<b>Exports</b><br/>jsonl · json · html"]
        REPO["<b>Repos</b><br/>git log"]
    end

    CRAWLERS & LOCAL --> SOURCEHARVEST
    SOURCEHARVEST == logspine.adapter.v1 JSONL ==> LOGSPINE
    AGENTTRAIL == logspine.adapter.v1 JSONL ==> LOGSPINE

    subgraph LOGSPINE_SURFACES [" Logspine surfaces "]
        STORE["<b>Store</b><br/>dedupe · index · relate"]
        BUNDLES["<b>Evidence bundles</b><br/>reviewable outputs"]
        SEARCH["<b>Search</b><br/>queries across imported evidence"]
    end

    LOGSPINE --> STORE
    LOGSPINE --> BUNDLES
    LOGSPINE --> SEARCH

    BOUNDARY["<b>Project boundary</b><br/>SourceHarvest reads local exports and does not crawl live services"]
    CRAWLERS -. already exported .-> BOUNDARY
    LOCAL -. local-only scan .-> BOUNDARY
    BOUNDARY -. limits .-> SOURCEHARVEST

    classDef source fill:#eff6ff,stroke:#2563eb,color:#1e3a8a;
    classDef adapter fill:#ecfdf5,stroke:#059669,color:#064e3b;
    classDef store fill:#fff7ed,stroke:#ea580c,color:#7c2d12;
    classDef guard fill:#fee2e2,stroke:#ef4444,color:#7f1d1d;
    class DISCRAWL,GITCRAWL,GRAINCRAWL,NOTCRAWL,SLACRAWL,TELECRAWL,NOTES,EXPORTS,REPO source;
    class SOURCEHARVEST,AGENTTRAIL adapter;
    class LOGSPINE,STORE,BUNDLES,SEARCH store;
    class BOUNDARY guard;
```

SourceHarvest is the non-agent source adapter layer. AgentTrail is the agent-session adapter layer. Logspine is the durable evidence layer.

```bash
sourceharvest markdown ./notes --source notes --collection notes:local --out - | spine import adapter -
agenttrail all --out - --redact safe | spine import adapter -
```

When `sourceharvest` is installed on `PATH`, Logspine can run it directly:

```bash
spine import sourceharvest markdown ./notes --source notes --collection notes:local --json
spine import sourceharvest gitlog . --source gitlog --collection repo:sourceharvest --json
```

For agent-session logs, use AgentTrail instead of SourceHarvest:

```bash
spine import agenttrail codex ~/.codex/sessions --json
spine import agenttrail hermes ~/.hermes/sessions --json
```

## Crawler Stack Boundary

SourceHarvest is the right home for adapters that read local crawler outputs and turn them into `logspine.adapter.v1` JSONL. It should not perform live service crawling itself.

Current crawler families to support through local adapters:

| Source | Domain | SourceHarvest role |
| --- | --- | --- |
| `discrawl` | Discord archives | Read local DB, snapshot, or export and emit adapter records. |
| `gitcrawl` | GitHub issues and pull requests | Read local archive or export and emit adapter records. |
| `graincrawl` | Granola notes and transcripts | Read local archive or export and emit adapter records. |
| `notcrawl` | Notion pages and databases | Read local archive or export and emit adapter records. |
| `slacrawl` | Slack messages and threads | Read local archive or export and emit adapter records. |
| `telecrawl` | Telegram Desktop archives | Read local archive or export and emit adapter records. |

These adapters should be added only from real local schemas or redacted sample exports. SourceHarvest scanner commands must stay local-only and must not make network calls.

## Build

```bash
go build -o bin/sourceharvest ./cmd/sourceharvest
go test ./...
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/solomonneas/sourceharvest/master/install.sh | sh
```

Or download a release binary and verify it with `checksums.txt`.

## Usage

Export generic JSONL records:

```bash
sourceharvest jsonl testdata/generic.fixture.jsonl \
  --source demo \
  --collection demo:collection \
  --out -
```

Export a Markdown directory as local note evidence:

```bash
sourceharvest markdown ./notes \
  --source notes \
  --collection notes:local \
  --out -
```

Export other local source shapes:

```bash
sourceharvest files ./notes \
  --source notes \
  --collection notes:files \
  --glob "*.md,*.txt" \
  --out -

sourceharvest html ./site-export \
  --source docs \
  --collection docs:html \
  --out -

sourceharvest gitlog . \
  --source gitlog \
  --collection repo:sourceharvest \
  --out -

sourceharvest json export.json \
  --source export \
  --collection export:records \
  --records-path records \
  --out -
```

Pipe into Logspine:

```bash
sourceharvest jsonl export.jsonl --source notes --collection notes:local --out - | spine import adapter -
sourceharvest markdown ./notes --source notes --collection notes:local --out - | spine import adapter -
```

Or let Logspine run SourceHarvest when `sourceharvest` is installed on `PATH`:

```bash
spine import sourceharvest markdown ./notes --source notes --collection notes:local --json
spine import sourceharvest gitlog . --source gitlog --collection repo:sourceharvest --json
```

## Boundary

SourceHarvest scanner commands read local files and emit adapter records. They do not make network calls.

Generated text is untrusted evidence, not instructions.
