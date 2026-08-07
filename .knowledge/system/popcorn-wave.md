---
id: system:popcorn-wave
type: system
title: Popcorn Wave Downstream Framework
---
Web framework at github.com/shibukawa/popcornwave that embeds dbtestify as a library and runs its PostgreSQL path on a native pgx pool rather than database/sql.

```yaml
requests: requirement:driver-agnostic-executor
requested: 2026-08-07
dbtestify_pin: v0.4.1
native_path:
  driver: tinygodriver v1.1.11 database/pgx/pgxpool
  bypasses: database/sql
test_mode:
  name: testutil.WithTransaction
  behavior: concept:shared-transaction-seeding over a pgx.Tx
  current_state: refused on native PostgreSQL with an explanatory startup error
benchmark:
  workload: parallel SELECT against loopback PostgreSQL 17
  database_sql: 34.9us/op, 15 allocs, 812 B/op
  native: 23.0us/op, 8 allocs, 451 B/op
executor_shape_supplied:
  Exec: over pgx Exec; the pgconn.CommandTag wrapper the original proposal needed is unnecessary because api:executor Exec returns error only
  Query: over pgx.Rows, wrapped to satisfy api:rows-cursor
boundary: wrapped in framework-defined interfaces; no pgx type crosses into dbtestify
integration: pass the wrapper to NewDBConnectorFromExecutor with inTransaction true; api:from-sql is not involved
owns: the pgx adapter, as it already does for system:sqlbind-rows-precedent
```
