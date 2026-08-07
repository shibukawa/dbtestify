---
id: api:from-sql
type: api
title: FromSQL database/sql Adapter
---
Adapts a database/sql handle to api:executor, which is how *sql.DB, *sql.Tx, and *sql.Conn reach dbtestify once the interface stops naming database/sql types.

```yaml
source: executor.go
signature: FromSQL(h SQLHandle) api:executor
sql_handle_interface:
  ExecContext: (ctx, query, args ...any) (sql.Result, error)
  QueryContext: (ctx, query, args ...any) (*sql.Rows, error)
satisfied_by: "*sql.DB, *sql.Tx, *sql.Conn"
adapter_behavior:
  Exec: calls ExecContext and drops the sql.Result
  Query: calls QueryContext and returns *sql.Rows as api:rows-cursor
typed_nil_guard: Query returns an untyped nil cursor on error rather than a (*sql.Rows)(nil) inside a non-nil interface
applied_internally_by:
  - NewDBConnectorFromDB
  - NewDBConnectorFromTx
  - Seed, when it opens its own transaction on the pool
caller_uses_it_for: NewDBConnectorFromExecutor with a database/sql handle
required_by: requirement:driver-agnostic-executor
```
