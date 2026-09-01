# Breviary — Agent Instructions

## Overview
Declarative runbook automation engine. Go 1.25, SQLite WAL, CEL expressions.

## Build & Test
- Build: `go build -o breviary ./cmd/breviary/`
- Test: `go test -count=1 ./...`
- Vet: `go vet ./...`
- Lint: `golangci-lint run ./...` (requires `make lint`)
- All: `make all`

## Code Standards
- **Go**: functional options, table-driven tests, context propagation, structured logging (slog)
- **Config**: env-parameterized, no hardcoded paths
- **Errors**: always checked (no `_` swallows), wrapped with `%w`
- **SQLite**: WAL mode, busy_timeout=5000, foreign_keys=ON, parameterized queries
- **YAML**: 2-space indent, no tabs, linted
- **Docker**: multi-stage, distroless, nonroot

## Commit Rules
- Format: conventional commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`
- Author: `Egor Kondrashov <kondrashov.woot@gmail.com>`
- No `--no-verify` EVER
- Pre-commit hooks must pass before commit
- Self-review: read own code before committing

## Quality Gates (Verifier)
Before every commit:
1. `make lint` — linter clean
2. `make vet` — no issues
3. `make test` — all pass
4. `make build` — compiles
5. No dead code (golangci-lint `unused`)
6. Table-driven tests for public API

## PR Workflow
- Branch: feature/name
- Open PR → CI runs test+docker+release
- Merge to main only after green CI
- Tag: `v*.*.*` for releases (GoReleaser)