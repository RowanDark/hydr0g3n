# Development Guide

## Pre-commit Hooks

This repository uses [`pre-commit`](https://pre-commit.com/) to enforce formatting and static analysis before changes are committed.

To install the hooks locally run:

```bash
pip install pre-commit
pre-commit install
```

The configured hooks will automatically run tools such as `gofmt`, `goimports`, and `go vet` along with basic repository hygiene checks.

You can manually run the hooks against all files with:

```bash
pre-commit run --all-files
```

## Continuous Integration

Pull requests are automatically validated through GitHub Actions. The CI workflow runs the Go unit tests as well as [`golangci-lint`](https://golangci-lint.run/) using the settings defined in `.golangci.yml`.
