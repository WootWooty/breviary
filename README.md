# Breviary — Declarative Runbook Automation Engine

[![CI](https://github.com/WootWooty/breviary/actions/workflows/ci.yml/badge.svg)](https://github.com/WootWooty/breviary/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/WootWooty/breviary)](https://goreportcard.com/report/github.com/WootWooty/breviary)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Breviary** is a declarative, GitOps-native runbook automation engine written in Go.  
Single binary, journal-first durable execution, no external dependencies.

## Install

```bash
# Linux/macOS — single binary
curl -L https://github.com/WootWooty/breviary/releases/latest/download/breviary_linux_amd64.tar.gz | tar xz
sudo mv breviary /usr/local/bin/

# Docker
docker pull ghcr.io/WootWooty/breviary:latest

# Homebrew
brew install WootWooty/tap/breviary
```

## Quick Start

```yaml
# hello.yaml
apiVersion: breviary.io/v1
kind: Runbook
metadata:
  name: hello
spec:
  steps:
    - id: greet
      action: exec
      exec: echo 'Hello, Breviary!'
    - id: done
      action: notify
      notify:
        channel: stdout
        msg: "Result: {{steps.greet.output}}"
```

```bash
breviary validate hello.yaml
breviary run hello.yaml
```

## CLI Reference

| Command | Description |
|---------|-------------|
| `breviary serve` | HTTP daemon (webhooks + approval API) |
| `breviary run <file>` | Execute a runbook |
| `breviary resume <run-id>` | Resume a failed run |
| `breviary validate <file>` | Validate runbook YAML |
| `breviary logs <run-id>` | Show audit trail |
| `breviary approve <run-id> <step-id>` | Approve a pending step |
| `breviary reject <run-id> <step-id>` | Reject a pending step |
| `breviary git <url> [ref]` | Sync runbooks from Git |

## Example Runbook

```yaml
apiVersion: breviary.io/v1
kind: Runbook
metadata:
  name: db-disk-space
  owner: db-team
spec:
  trigger:
    alert: DiskUsageHigh
    severity: critical
    concurrency: 1
    dedup: 5m

  steps:
    - id: check-usage
      action: exec
      exec: df -h /var/lib/postgresql
    
    - id: vacuum
      action: exec
      exec: psql -d app -c "VACUUM ANALYZE"
      approval:
        channel: telegram
        timeout: 30m
        escalate_after: 10m
        show: "Run VACUUM ANALYZE on production database?"
      undo:
        id: undo-vacuum
        action: exec
        exec: psql -d app -c "VACUUM FULL"
    
    - id: notify-ok
      action: notify
      notify:
        channel: telegram
        msg: "Done: {{steps.check-usage.output}}"
```

## Architecture

```
breviary/
├── cmd/breviary/main.go          # CLI entry point
├── internal/
│   ├── spec/        # YAML → model, JSON Schema
│   ├── expr/        # CEL expressions for when-conditions
│   ├── engine/      # Core: journal-first executor
│   ├── actions/     # Registry: exec/http/script/notify
│   ├── journal/     # SQLite WAL, audit trail, pinned spec
│   ├── config/      # ~/.config/breviary/config.yaml
│   ├── server/      # HTTP daemon (webhook + approval API)
│   ├── templates/   # Go templates {{steps.X.output}}
│   ├── git/         # GitOps synchronization
│   └── ...
├── Dockerfile
├── .goreleaser.yml
└── LICENSE (Apache 2.0)
```

## Features

- **Journal-first**: checkpoint before side-effect → crash-safe resume
- **Retry**: built-in exponential backoff
- **Saga rollback**: undo steps in reverse order
- **Trigger guard**: dedup/throttle/concurrency (storm protection)
- **Approval**: Telegram/HTTP, escalation, timeout
- **CEL `when`**: conditional step execution
- **Secrets**: `${ENV}` only, masked in logs
- **GitOps**: `breviary git pull` — runbooks as code
- **Single binary**: static compilation, ~17MB, CGo-free

## License

Apache 2.0. See [LICENSE](LICENSE).