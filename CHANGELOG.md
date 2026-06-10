# Changelog

All notable changes to SourceHarvest are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `gitlog` reader now captures the full commit body (appended to `item.text`
  after the subject), the author email (on `actor.metadata.email`), and the list
  of changed files (as `file` artifacts and in `item.metadata.changed_files`).
  Empty bodies and merge commits are handled gracefully.
- `markdown` reader parses an optional leading YAML front-matter block. `title`
  sets the item title, `date` sets `created_at` when parseable, `tags` are added
  to the item tags, and `author` becomes a human actor. The front-matter block is
  stripped from `item.text` and any other scalar keys are preserved under
  `item.metadata.front_matter`. Parsing is stdlib-only with no YAML dependency,
  and malformed front-matter falls back to the previous plain-text behavior.

### Fixed

- Export readers (`files`, `html`, `gitlog`, `json`) now read their flag values
  after parsing instead of before, and an invalid `--limit` value surfaces a
  clear error instead of being silently swallowed and defaulting to 0.

## [0.1.1] - 2026-06-03

### Changed

- Hardened export writes (atomic, owner-only output files) and documented the
  crawler stack boundary.
- Verified the installed release asset during installation.

## [0.1.0] - 2026-06-03

### Added

- Initial release of the SourceHarvest exporter.
- Local file readers: `jsonl`, `json`, `markdown`, `files`, `html`, and `gitlog`,
  each emitting `miseledger.adapter.v1` JSONL.
- Optional JSON summary with record counts, file counts, warnings, and a
  generated timestamp; `--limit` bounds and per-reader filters.

[Unreleased]: https://github.com/escoffier-labs/sourceharvest/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/escoffier-labs/sourceharvest/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/escoffier-labs/sourceharvest/releases/tag/v0.1.0
