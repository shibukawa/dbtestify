---
id: requirement:driver-agnostic-executor
type: requirement
title: Driver-Agnostic Executor for Non-database/sql Backends
---
dbtestify must run Seed and Assert through an executor backed by a driver outside database/sql, so a caller holding one open native transaction can seed, exercise, and assert inside it.

```yaml
requester: system:popcorn-wave
requested: 2026-08-07
target: github.com/shibukawa/dbtestify
downstream_pin: v0.4.1
precedent: system:sqlbind-rows-precedent
status: implemented
problem:
  blocker: api:executor required the concrete return types *sql.Rows and *sql.Row
  cause: both are structs with unexported fields; only database/sql can construct them
  effect: a non-database/sql backend satisfied ExecContext but never QueryContext or QueryRowContext
  downstream_regression: system:popcorn-wave refuses WithTransaction on native PostgreSQL with an explanatory startup error
  rejected_workaround: a second database/sql connection cannot observe uncommitted state; see concept:shared-transaction-seeding
design:
  - api:executor
  - api:rows-cursor
  - api:from-sql
  - rule:internal-query-call-sites
shape_decision: decision:driver-agnostic-executor-shape
backward_compatibility: waived by the requester on 2026-08-07, which is what allows the single-interface design
non_goals:
  - pgx adapter inside dbtestify; system:popcorn-wave owns the adapter as it already does for system:sqlbind-rows-precedent
  - changes to YAML dataset format, dialects, seed strategies, CLI, or httpapi
  - transaction management; the caller opens the transaction and passes the in-transaction executor
  - a native pool that Seed can open its own transaction on; a poolless connector still requires inTransaction
acceptance:
  - an executor implementing only Exec and Query runs Seed under clear-insert and upsert, the primary-key-lookup strategy, against a caller-opened transaction
  - the same executor runs Assert and observes its own uncommitted rows
  - every cursor dbtestify opens is closed, asserted by counting unclosed cursors in the fake native executor
  - catalog reads TableNames and PrimaryKeys resolve through the native cursor
  - connect, httpapi, httpapi/serve, assertdb, and examples compile and pass unchanged
  - downstream proof, still pending: system:popcorn-wave WithTransaction on native PostgreSQL seeds, serves requests, asserts, then rolls back inside one shared pgx transaction
verification:
  test: executor_native_test.go
  fake_backend: implements api:executor over a caller-opened *sql.Tx and returns its own cursor type, distinct from *sql.Rows, so a path still reaching for *sql.Rows would fail
  suite: full go test ./... green
  former_flake: TestAssertSQLite and TestSeedSQLite failed intermittently while this work was in progress; the cause predated it and is now fixed under rule:test-pool-teardown
```
