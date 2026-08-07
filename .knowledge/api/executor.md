---
id: api:executor
type: api
title: Executor Interface
---
The statement target dbtestify runs everything through, named entirely in dbtestify types so a backend that bypasses database/sql implements it directly.

```yaml
source: executor.go
methods:
  Exec: (ctx, query, args ...any) error
  Query: (ctx, query, args ...any) (api:rows-cursor, error)
database_sql_types_referenced: none
exec_returns_error_only:
  reason: all 12 internal Exec call sites discarded sql.Result; LastInsertId and RowsAffected appear nowhere in non-test code
  effect: a native backend needs no sql.Result wrapper around its command tag
query_row_context: removed; the scalar reads became one-row cursor reads per rule:internal-query-call-sites
database_sql_backends: reach it through api:from-sql, not by direct satisfaction
implemented_by: any backend supplying two methods; see decision:driver-agnostic-executor-shape
required_by: requirement:driver-agnostic-executor
```
