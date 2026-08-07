---
id: decision:unknown-affected-sentinel
type: decision
title: Unknown Affected-Row Count Is -1, Not 0
---
Resolved: a backend that cannot report an affected-row count returns -1, exported as UnknownAffected, because 0 would be indistinguishable from a statement that genuinely matched no rows.

```yaml
state: resolved
resolved: 2026-08-07
resolved_by: dbtestify owner
constant: UnknownAffected int64 = -1
rejected: returning 0, which silently claims "no rows matched" for a driver that simply does not count
where_unknown_arises:
  - a driver whose sql.Result.RowsAffected returns an error
  - a native backend with no count to give
  - the opening call of api:seed-callback, before the task has run
error_handling: api:from-sql swallows the RowsAffected error and reports the sentinel, because a statement that already succeeded must not fail over a value used only for reporting
propagation_rule: addAffected keeps a sum unknown when any contribution is unknown, so a partially countable batch never reports a misleadingly small total
consumer_contract: check for a negative count rather than comparing against -1 exactly
decides: requirement:affected-row-reporting
```
