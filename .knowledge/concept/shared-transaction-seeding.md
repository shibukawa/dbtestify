---
id: concept:shared-transaction-seeding
type: concept
title: Shared-Transaction Seeding and Assertion
---
Test mode in which seeding, the code under test, and assertion all run on one open transaction that is rolled back afterward, so fixtures never reach committed state.

```yaml
participants: dbtestify Apply, the request under test, dbtestify Assert
invariant: all three observe the same uncommitted state
isolation_consequence: a second connection cannot observe another transaction's uncommitted writes under any isolation level
implication: dbtestify must accept the caller's in-transaction executor; opening its own connection is not a substitute
teardown: caller rolls back; no cleanup statements needed
native_case: on a non-database/sql driver the shared transaction is a native handle such as pgx.Tx, which api:executor now accepts once wrapped
database_sql_case: NewDBConnectorFromTx, or NewDBConnectorFromExecutor over api:from-sql with inTransaction true
pool_contrast: a connector with no pool cannot open its own transaction, so Seed on one requires inTransaction
downstream: system:popcorn-wave testutil.WithTransaction
motivates: requirement:driver-agnostic-executor
```
