---
id: api:seed-callback
type: api
title: SeedOpt Callback
---
Progress hook Seed invokes twice per table task, once on entry and once on completion, carrying the affected-row count and the error.

```yaml
source: seed.go
signature: func(targetTable, task string, start bool, rows int64, err error)
tasks: truncate, insert, upsert, delete
call_pairs: one opening call with start true, one closing call with start false
rows_on_open: always the sentinel from decision:unknown-affected-sentinel, since the task has not run
rows_on_close: the summed count, or the sentinel
err_on_close: the task error, or nil
consumers:
  - cmd/dbtestify, which prints the count beside the elapsed time and omits it when unknown
  - httpapi SeedTableResult, which exposes it as a rows field
required_by: requirement:affected-row-reporting
```
