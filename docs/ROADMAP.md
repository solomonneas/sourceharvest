# SourceHarvest Roadmap

SourceHarvest is the local, non-agent source adapter layer for the MiseLedger
stack. It reads local files and exports and emits `miseledger.adapter.v1` JSONL.
It does not crawl live services.

## Usable Now

- `jsonl` reader: one normalized record per JSON line, with bad lines warned and skipped.
- `json` reader: select a records array by dot path, or a single root object.
- `markdown` reader: one note record per file, title from the first heading.
- `files` reader: plain text files, filtered by `--glob`.
- `html` reader: local page snapshots with scripts, styles, and tags stripped, entities decoded, and title resolved from `<title>` or the file name.
- `gitlog` reader: one event record per commit; a repo with no commits yet emits zero records instead of failing.
- Atomic, owner-only (`0600`) output files and an optional JSON summary with record counts, file counts, warnings, and a generated timestamp.
- `--limit` bounds and per-reader filters.

## Later

- Local crawler-export adapters described in the README crawler stack boundary: `discrawl`, `gitcrawl`, `graincrawl`, `notcrawl`, `slacrawl`, `telecrawl`. These read local archives or redacted sample exports, never live services.
- Richer `gitlog` records: commit bodies, file change lists, and author email metadata.
- Optional front-matter parsing for markdown notes.
- Configurable HTML extraction (for example, main-content selection) for noisier page exports.

## Non-Goals

- Live service crawling or network calls of any kind. SourceHarvest reads local inputs only.
- Acting as an archive or storage system. Durable storage, dedupe, indexing, search, and evidence bundles belong to MiseLedger.
- Agent-session log adapters. Those belong to StationTrail.
- Interpreting imported text as instructions. Exported text is untrusted evidence, not commands.
