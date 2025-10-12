# Contributing to hydr0g3n

Thanks for taking the time to contribute! This guide walks you through the workflow we use for issues, pull requests, and code changes.

## Code of Conduct

Participation in this project is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). By contributing you agree to uphold it.

## Getting Started

1. Fork the repository and create a feature branch from `main`.
2. Make sure you have Go 1.24 or newer installed (see `go.mod`).
3. Install the development tooling listed in [docs/development.md](docs/development.md).
4. Run `go test ./...` and `golangci-lint run` (or `pre-commit run --all-files`) locally before submitting a pull request.

## Making Changes

- Keep changes focused. Separate unrelated updates into different branches and pull requests.
- Follow Go best practices and standard library conventions. Run `gofmt` on any Go files you touch.
- Update or add tests alongside your changes whenever possible.
- If your change affects user-facing behavior or documentation, update the relevant docs.

## Commit Messages

- Use clear, descriptive commit messages written in the imperative mood (e.g. "Add CLI option for ...").
- Reference related issues with `Fixes #123` (or similar) in the pull request description when appropriate.

## Pull Requests

- Fill out the pull request template. It includes a checklist of reminders to help reviewers.
- Include screenshots for visual/UI changes when applicable.
- Be responsive to review feedback; discussions help maintain quality and shared understanding.

Thank you for helping improve hydr0g3n!
