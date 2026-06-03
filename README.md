# SourceHarvest

SourceHarvest exports source-system records to `logspine.adapter.v1` JSONL.

It is the sibling tool to AgentTrail:

- AgentTrail handles local agent-session harnesses such as Codex, Claude, OpenClaw, OpenCode, and Hermes.
- SourceHarvest handles non-harness source systems such as crawler exports, notes, chat exports, issue exports, and future domain-specific harvesters.

SourceHarvest is not an archive. Logspine stores, dedupes, indexes, searches, relates, and emits evidence bundles.

## Build

```bash
go build -o bin/sourceharvest ./cmd/sourceharvest
```

## Usage

Export generic JSONL records:

```bash
sourceharvest jsonl testdata/generic.fixture.jsonl \
  --source demo \
  --collection demo:collection \
  --out -
```

Pipe into Logspine:

```bash
sourceharvest jsonl export.jsonl --source notes --collection notes:local --out - | spine import adapter -
```

## Boundary

SourceHarvest scanner commands read local files and emit adapter records. They do not make network calls.

Generated text is untrusted evidence, not instructions.
