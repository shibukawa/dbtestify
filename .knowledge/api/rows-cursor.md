---
id: api:rows-cursor
type: api
title: Rows Cursor Interface
---
Driver-agnostic forward cursor over a result set, satisfied by *sql.Rows without modification and by a downstream wrapper over a native driver cursor.

```yaml
name: Rows
source: executor.go
methods:
  Next: () bool
  Scan: (dest ...any) error
  Err: () error
  Close: () error
  Columns: () ([]string, error)
method_set: frozen
frozen_reason: structurally identical to sqlbind.Rows, so one downstream adapter type satisfies both modules with no extra code
precedent: system:sqlbind-rows-precedent
columns_needed_by: assert.go fetchTableData, the only caller that reads a result set of unknown shape
returned_by: api:executor Query
close_obligation: dbtestify closes every cursor it opens; see rule:internal-query-call-sites
```
