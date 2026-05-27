# Contributing

Thank you for your interest in contributing to the Atlassian OpenTofu Provider.

## Getting Started

1. Fork the repository
2. Clone your fork
3. Create a feature branch from `main`
4. Install dependencies: `go mod tidy`
5. Set up git hooks: `ln -sf ../../git-hooks/pre-commit .git/hooks/pre-commit && ln -sf ../../git-hooks/pre-push .git/hooks/pre-push`

## Development Requirements

- Go >= 1.26
- Docker (for mock API)
- OpenTofu >= 1.6

## Coding Standards

All code must comply with the [Asymmetric Effort Coding Standards](https://coding-standards.asymmetric-effort.com/#/getting-started).

Key requirements:

- All public functions and types must have GoDoc comments
- No unused imports, variables, or dead code
- No unresolved TODO/FIXME comments in shipped code
- No hardcoded secrets or credentials
- No recursion (Go does not guarantee tail-call optimization)
- Bounded queues and buffers; prefer pure functions

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` — new feature
- `fix:` — bug fix
- `test:` — adding or updating tests
- `docs:` — documentation changes
- `refactor:` — code restructuring without behavior change
- `perf:` — performance improvement
- `chore:` — maintenance tasks

## Testing

- Tests are organized in `test/unit/`, `test/integration/`, and `test/e2e/`
- Coverage threshold: >= 98%
- Run tests: `make test`
- Run coverage: `make cover`

## Pull Requests

- All PRs require at least one reviewer approval
- All review comments must be resolved before merge
- CI must pass (lint, test, build, coverage)
- CodeQL must report no high/critical findings
