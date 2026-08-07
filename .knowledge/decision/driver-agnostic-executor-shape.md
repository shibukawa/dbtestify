---
id: decision:driver-agnostic-executor-shape
type: decision
title: Redefine Executor Instead of Layering an Optional Interface
---
Resolved: api:executor was redefined to a two-method driver-agnostic shape rather than kept intact behind an optional-interface upgrade, because the requester waived backward compatibility.

```yaml
state: resolved
resolved: 2026-08-07
resolved_by: requester waived backward compatibility and asked for the simplest implementation
supersedes:
  - the optional RowsQuerier interface originally proposed
  - the Query dispatch function that would have type-asserted for it
  - the UnimplementedQuerier embeddable stub
  - the open question of whether to drop QueryRowContext
what_was_dropped:
  QueryRowContext: no internal caller remained after rule:internal-query-call-sites
  sql.Result: every internal call site discarded it; Exec now returns error only
  runtime_type_assertion: one interface means one code path, no dispatch branch
enabling_fact: the public entry points NewDBConnectorFromDB and NewDBConnectorFromTx take concrete *sql.DB and *sql.Tx, so redefining Executor left them source-compatible
breaking_changes:
  - a caller passing a database/sql handle to NewDBConnectorFromExecutor must wrap it with api:from-sql
  - NewDBConnectorFromExecutor no longer detects a *sql.DB as a pool, so Seed outside a transaction requires NewDBConnectorFromDB
  - a third party implementing api:executor must supply the new two methods
blast_radius_measured: only dbconnector_tx_test.go needed edits; connect, httpapi, httpapi/serve, assertdb, and examples compiled unchanged
rejected_alternative: widening QueryContext in place, which Go's lack of return covariance forbids while *sql.DB must still satisfy the interface
decides: requirement:driver-agnostic-executor
```
