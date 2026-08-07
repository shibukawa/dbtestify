---
id: requirement:affected-row-reporting
type: requirement
title: Report Affected Rows During Seeding
---
Seeding must report how many rows each task affected so a run can log it, while keeping an unreported count distinguishable from a task that matched nothing.

```yaml
requested: 2026-08-07
requested_by: dbtestify owner, while reviewing requirement:driver-agnostic-executor
status: implemented
motivation: a seed run that says only "OK" cannot tell an operator whether the fixture actually landed
sentinel: decision:unknown-affected-sentinel
chain:
  - api:executor Exec returns the count
  - DBConnector Insert, Delete, Upsert, and Truncate return it
  - api:seed-callback carries it to the caller
  - cmd/dbtestify prints "N rows" beside the elapsed time
  - httpapi SeedTableResult gains a rows field, in both JSON and text output
accumulation: batches sum through addAffected, which yields unknown when any batch is unknown
defects_fixed_in_passing:
  insert_upsert_callback_error: the closing callback passed a literal nil instead of the error, so a failed insert or upsert reported success
  delete_callback_missing: DeleteOperation never invoked the callback at all, leaving the CLI delete branch unreachable
acceptance:
  - a backend outside database/sql reports a real count that reaches the callback
  - a truncate over an empty table reports 0, not the unknown sentinel
  - a backend that cannot count reports the sentinel end to end
verification: executor_native_test.go TestNativeExecutorReportsAffectedRows, TestUnknownAffectedStaysNegative, TestAddAffected
```
