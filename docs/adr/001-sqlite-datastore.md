# ADR-001: SQLite as Primary Datastore

**Date:** 2026-09-01  
**Status:** Accepted  

## Context

Breviary is a single-node CLI/daemon DevOps tool for runbook automation. It needs:
- Persistent journal for audit trail (step events, run history)
- Concurrent readers + occasional writer (single-user or webhook-triggered)
- Zero infrastructure dependencies (no Postgres server to maintain)
- Embedded, portable binary (CGo-free via modernc.org/sqlite)

Alternatives considered:
- **Postgres**: Overkill for single-node. Adds deployment complexity (connection string, auth, migration)
- **BoltDB**: Excellent embedded KV, but lacks SQL queryability for audit reports
- **JSON file**: No transactional guarantees, no concurrent safety
- **YAML file**: Same as JSON — no ACID

## Decision

Use **SQLite in WAL mode** via `modernc.org/sqlite` (pure Go, CGo-free).

Key PRAGMAs on every connection:
```sql
PRAGMA journal_mode = WAL;        -- concurrent readers + writer
PRAGMA busy_timeout = 5000;       -- 5s retry on lock
PRAGMA synchronous = NORMAL;      -- balance speed/safety
PRAGMA foreign_keys = ON;         -- referential integrity
PRAGMA cache_size = -64000;       -- 64MB page cache
```

## Consequences

**Good:**
- Single-file storage — backup = `cp breviary.db backup.db`
- Zero infrastructure — works out of the box
- ~10-50K writes/sec on NVMe — sufficient for single-node
- Journal-first pattern (checkpoint before side-effect, record after) maps to SQLite transactions

**Trade-offs:**
- Single writer at a time — WAL mitigates for read-concurrent workloads
- WAL file can grow on high-write workloads — need periodic `wal_checkpoint(TRUNCATE)`
- Not suitable for multi-server deployments — if needed, migrate to Postgres (via repository abstraction layer in v2)

## Related

- Internal journal package: `internal/journal/store.go`