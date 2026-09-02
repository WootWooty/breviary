# Contributing to Breviary

Thank you for considering contributing to Breviary! This document outlines the workflow, standards, and conventions.

## Quick Start

```bash
git clone https://github.com/WootWooty/breviary.git
cd breviary
go build -o breviary ./cmd/breviary/
go test -count=1 ./...
```

## Development Workflow

1. **Branch**: create a feature branch from `main`
2. **Commits**: atomic conventional commits (see below)
3. **PR**: open a pull request against `main`
4. **CI**: must pass all checks (lint, vet, test, build)
5. **Merge**: squash & merge after green CI

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add parallel step execution
fix: prevent nil pointer on empty output
refactor: extract approval manager
test: add golden file comparison for validate command
docs: explain dedup configuration
chore: update GoReleaser to v2.4
```

## Code Standards

- **Go 1.25** — use latest idioms (range-over-func, slog, context.AfterFunc)
- **Error wrapping**: `fmt.Errorf("context: %w", err)` — always wrap
- **Context**: propagate `context.Context` as first parameter
- **Logging**: use `slog` — no `log.Printf` / `fmt.Println`
- **Tests**: table-driven (`t.Run`), parallel (`t.Parallel`) where possible
- **SQLite**: WAL mode, `busy_timeout=5000`, `foreign_keys=ON`
- **YAML**: 2-space indent, no tabs

## Project Structure

```
cmd/breviary/    — CLI entry point (thin — dispatch only)
internal/
  spec/          — YAML model & validation
  engine/        — journal-first runbook executor
  actions/       — exec/script/notify/approve registry
  journal/       — SQLite audit trail
  server/        — HTTP webhook + approval API
  expr/          — CEL expression evaluation
  config/        — ~/.config/breviary/config.yaml
  templates/     — Go template rendering
  git/           — GitOps sync
test/harness/    — acceptance tests (black-box binary)
```

## Testing

```bash
# All tests
go test -count=1 ./...

# With coverage
go test -count=1 -coverprofile=cover.out ./...
go tool cover -func=cover.out

# Update golden files after intentional output changes
go test -count=1 -update ./test/harness/

# Lint (requires golangci-lint)
golangci-lint run --config .golangci.yml ./...
```

## Pull Request Checklist

Before opening a PR:

- [ ] `make lint` passes
- [ ] `go vet ./...` is clean
- [ ] `go test -count=1 ./...` passes
- [ ] `go build ./cmd/breviary/` compiles
- [ ] New behaviour has tests
- [ ] Golden files updated (if output changed)
- [ ] `README.md` updated (if user-facing changes)

## Reporting Issues

- Include the full command and output
- Attach the runbook YAML (if applicable)
- Mention your OS and Go version

## License

By contributing, you agree that your contributions will be licensed under Apache 2.0.