---
id: api:executor
type: api
title: Executor Interface
---
The statement target dbtestify runs everything through, named entirely in dbtestify types so a backend that bypasses database/sql implements it directly.

```yaml
source: executor.go
methods:
  Exec: (ctx, query, args ...any) (int64, error)
  Query: (ctx, query, args ...any) (api:rows-cursor, error)
database_sql_types_referenced: none
exec_returns_count_not_sql_result:
  reason: sql.Result was dead weight since all 12 call sites discarded it, but the count itself is wanted for requirement:affected-row-reporting
  effect: a native backend returns a plain int64, needing no sql.Result wrapper; pgconn.CommandTag already exposes RowsAffected directly
  unknown: decision:unknown-affected-sentinel
query_row_context: removed; the scalar reads became one-row cursor reads per rule:internal-query-call-sites
database_sql_backends: reach it through api:from-sql, not by direct satisfaction
implemented_by: any backend supplying two methods; see decision:driver-agnostic-executor-shape
required_by: requirement:driver-agnostic-executor
```
