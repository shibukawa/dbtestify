---
id: rule:internal-query-call-sites
type: rule
title: Internal Reads Route Through Executor Helpers
---
Every internal read runs through api:executor and closes its cursor; the six catalog reads and four scalar reads collapsed into two helpers rather than being rewritten one by one.

```yaml
verified_at: v0.4.1 source, migrated in this change
helpers:
  queryStrings: drains a single-string-column result into a slice; used by all six catalog reads
  queryString: reads one row holding one string; replaces every QueryRowContext scalar read
  both: defer Close and return rows.Err()
migrated:
  table_scan:
    - assert.go fetchTableData SELECT * FROM <table>, already had defer Close
  catalog_reads:
    - psqlDBConnector.TableNames
    - psqlDBConnector.PrimaryKeys
    - mysqlDBConnector.TableNames
    - mysqlDBConnector.PrimaryKeys
    - sqliteDBConnector.TableNames
    - sqliteDBConnector.PrimaryKeys
  scalar_reads:
    - postgres SELECT current_schema() x2
    - mysql SELECT DATABASE() x2
  exec_sites: 12 ExecContext calls became Exec, each collapsing to a single return
defects_fixed_in_passing:
  cursor_leak: the six catalog loops never called Close and never checked Err, leaking on the mid-loop error return
  mysql_schema_literal: mysqlDBConnector.TableNames passed the literal "foo" as the schema argument instead of the resolved schema name
postcondition: no QueryRowContext and no *sql.Rows outside api:from-sql
required_by: requirement:driver-agnostic-executor
```
