---
id: system:sqlbind-rows-precedent
type: system
title: tinybind-go sqlbind Driver-Agnostic Rows Precedent
---
The sqlbind package of tinybind-go, which shipped the same optional-interface upgrade in v0.4.2 for the same downstream on the same date; dbtestify adopts its shape so one adapter serves both modules.

```yaml
module: tinybind-go
package: sqlbind
shipped: v0.4.2
upstream_requirement_slug: sql-driver-agnostic-rows
upstream_requirement_note: lives in the tinybind-go catalog, not this one
requested_by: system:popcorn-wave
requested: 2026-08-07
shape_reused:
  - sqlbind.Rows equals api:rows-cursor method for method
compatibility_constraint: keep api:rows-cursor structurally identical, so the downstream pgx.Rows wrapper satisfies both modules with no extra code
divergence:
  sqlbind: kept its interface intact and layered an optional querier plus an embeddable stub, preserving backward compatibility
  dbtestify: redefined api:executor outright per decision:driver-agnostic-executor-shape, since the requester waived compatibility
  shared: only the cursor shape, which is the part the downstream adapter depends on
adapter_ownership: downstream, not the library
informs: requirement:driver-agnostic-executor
```
