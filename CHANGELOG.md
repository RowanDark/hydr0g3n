# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

- _No changes yet._

## Preparing a Release

Run the helper below before tagging a new release to capture merged pull requests
since the previous tag and seed the release notes template:

```bash
scripts/release-notes.sh <previous-tag> [<new-ref>]
```

The script prints a Markdown template to standard output that can be copied into
this changelog or a GitHub release description.
