---
id: rule:test-pool-teardown
type: rule
title: Test Pools Close Through t.Cleanup
---
A test that opens a database pool must close it through t.Cleanup, never from a goroutine watching a context, so teardown finishes before the next subtest starts.

```yaml
helper: openTestDBConnector in testutil_test.go, which now takes *testing.T
forbidden_pattern: "go func() { <-ctx.Done(); db.Close() }()"
why:
  race: several subtests os.Remove a SQLite file and reopen the same cache=shared DSN
  effect: the previous subtest's asynchronous close overlapped the next one's remove and open
  symptom: intermittent "attempt to write a readonly database"
  scope: it hit any subtest in the run, not a fixed one, which is why it read as unrelated noise
operation_context: subtests pass t.Context() for queries; the cancel-based context existed only to drive the old close goroutine
evidence:
  before: 3 of 5 attempts failed at go test -count=10 -run 'TestAssertSQLite|TestSeedSQLite'
  after: 0 of 5 at -count=10, 0 of 3 at -count=30, 0 of 6 full suite runs
  baseline_check: the same 3 of 5 rate on the pre-change merge-base, confirming it predated requirement:driver-agnostic-executor
applies_to: assert_test.go, seed_test.go, dbconnector_test.go
```
