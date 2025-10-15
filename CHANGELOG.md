# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

- _No changes yet._

## [1.0.0] - 2024-06-06

### Added

- Added an embedding API so other tooling can run Hydr0g3n programmatically (#34).
- Expanded the Burp Suite export pipeline and outbound API integration to streamline triage (#33).
- Introduced end-to-end integration tests that exercise the local server workflow (#32).
- Delivered a benchmarking harness for tracking scanner performance over time (#31).
- Added a scan safety banner and opt-in flag for sensitive targets (#30).
- Required an explicit legal confirmation before enabling aggressive scans (#30).

## Preparing a Release

Run the helper below before tagging a new release to capture merged pull requests
since the previous tag and seed the release notes template:

```bash
scripts/release-notes.sh <previous-tag> [<new-ref>]
```

The script prints a Markdown template to standard output that can be copied into
this changelog or a GitHub release description.
