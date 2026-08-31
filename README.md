# Breviary — Runbook Automation Engine

> Декларативный GitOps-native runbook automation engine.
> Go, один бинарник, journal-first durable execution.

[![CI](https://github.com/egor-romanov/breviary/actions/workflows/ci.yml/badge.svg)](https://github.com/egor-romanov/breviary/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/egor-romanov/breviary)](https://goreportcard.com/report/github.com/egor-romanov/breviary)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Установка

```bash
# Linux/macOS — один бинарник
curl -L https://github.com/egor-romanov/breviary/releases/latest/download/breviary_linux_amd64.tar.gz | tar xz
sudo mv breviary /usr/local/bin/

# Docker
docker pull ghcr.io/egor-romanov/breviary:latest

# Homebrew
brew install egor-romanov/tap/breviary
```

## Быстрый старт

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

## Команды

| Команда | Описание |
|---------|----------|
| `breviary serve` | HTTP daemon (webhooks + approval API) |
| `breviary run <file>` | Запустить runbook |
| `breviary resume <run-id>` | Продолжить прерванный run |
| `breviary validate <file>` | Проверить YAML |
| `breviary logs <run-id>` | Audit trail |
| `breviary approve <run-id> <step-id>` | Одобрить шаг |
| `breviary reject <run-id> <step-id>` | Отклонить шаг |
| `breviary git <url> [ref]` | Синхронизировать runbook из Git |

## Пример runbook

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
        show: "Запустить VACUUM ANALYZE на production?"
      undo:
        action: exec
        exec: psql -d app -c "VACUUM FULL"

    - id: notify-ok
      action: notify
      notify:
        channel: telegram
        msg: "Готово: {{steps.check-usage.output}}"
```

## Архитектура

```
breviary/
├── cmd/breviary/main.go          # CLI entry
├── internal/
│   ├── spec/        # YAML → модель, JSON Schema
│   ├── expr/        # CEL-выражения для when-условий
│   ├── engine/      # Ядро: journal-first executor
│   ├── actions/     # Registry: exec/http/script/notify
│   ├── journal/     # SQLite WAL, audit trail, pinned spec
│   ├── config/      # ~/.config/breviary/config.yaml
│   ├── server/      # HTTP daemon (webhook + approval API)
│   ├── templates/   # Go-шаблоны {{steps.X.output}}
│   ├── git/         # GitOps-синхронизация
│   └── ...
├── Dockerfile
├── .goreleaser.yml
└── LICENSE (Apache 2.0)
```

## Возможности

- **Journal-first**: checkpoint ДО side-effect → crash-safe resume
- **Retry**: встроенный exponential backoff
- **Saga rollback**: undo-шаги в обратном порядке
- **Trigger guard**: dedup/throttle/concurrency (защита от шторма)
- **Approval**: Telegram/HTTP inline-кнопки, эскалация, timeout
- **CEL `when`**: условное выполнение шагов
- **Secrets**: только ${ENV}, маскирование в логах
- **GitOps**: `breviary git pull` — runbook как код
- **One binary**: статическая компиляция, ~17MB, CGo-free

## Лицензия

Apache 2.0. See [LICENSE](LICENSE).