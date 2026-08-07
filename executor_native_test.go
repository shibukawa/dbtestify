package dbtestify

import (
	"context"
	"database/sql"
	"testing"

	"github.com/alecthomas/assert/v2"
	_ "github.com/mattn/go-sqlite3"
)

// nativeExecutor stands in for a backend that bypasses database/sql, such as a
// pgx pool. It returns its own cursor type, so any dbtestify path that still
// expected *sql.Rows would fail here rather than silently work.
//
// It runs on a caller-opened transaction, which is the shape a shared
// test-transaction gives dbtestify.
type nativeExecutor struct {
	tx      *sql.Tx
	cursors []*nativeRows
}

var _ Executor = (*nativeExecutor)(nil)

func (e *nativeExecutor) Exec(ctx context.Context, query string, args ...any) error {
	_, err := e.tx.ExecContext(ctx, query, args...)
	return err
}

func (e *nativeExecutor) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := e.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	cursor := &nativeRows{rows: rows}
	e.cursors = append(e.cursors, cursor)
	return cursor, nil
}

func (e *nativeExecutor) openCursors() int {
	var open int
	for _, cursor := range e.cursors {
		if !cursor.closed {
			open++
		}
	}
	return open
}

// nativeRows is deliberately a distinct type from *sql.Rows.
type nativeRows struct {
	rows   *sql.Rows
	closed bool
}

var _ Rows = (*nativeRows)(nil)

func (r *nativeRows) Next() bool                 { return r.rows.Next() }
func (r *nativeRows) Scan(dest ...any) error     { return r.rows.Scan(dest...) }
func (r *nativeRows) Err() error                 { return r.rows.Err() }
func (r *nativeRows) Columns() ([]string, error) { return r.rows.Columns() }

func (r *nativeRows) Close() error {
	r.closed = true
	return r.rows.Close()
}

// TestNativeExecutorSeedsAndAsserts is the point of the driver-agnostic
// Executor: seeding, the code under test, and assertion all share one
// transaction that dbtestify never opened and cannot see through database/sql.
func TestNativeExecutorSeedsAndAsserts(t *testing.T) {
	db := memberDB(t, "native_executor_test")

	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	exec := &nativeExecutor{tx: tx}
	dbc, err := NewDBConnectorFromExecutor(exec, SQLiteDialect, true)
	assert.NoError(t, err)
	assert.True(t, dbc.InTransaction())

	assert.NoError(t, Seed(t.Context(), dbc, memberDataSet("Frank", "Grace"), SeedOpt{}))

	// Upsert reads primary keys from the catalog, so it exercises the
	// key-lookup path through the native cursor as well.
	assert.NoError(t, Seed(t.Context(), dbc, memberDataSet("Frank", "Heidi"), SeedOpt{
		Operations: map[string]Operation{"member": UpsertOperation},
	}))

	ok, _, err := Assert(t.Context(), dbc, memberDataSet("Frank", "Heidi"), AssertOpt{})
	assert.NoError(t, err)
	assert.True(t, ok, "native executor must observe its own uncommitted rows")

	assert.True(t, len(exec.cursors) > 0, "the run must have opened cursors")
	assert.Equal(t, 0, exec.openCursors(), "dbtestify must close every cursor it opens")

	// Nothing was committed, so the pool never saw the rows.
	assert.NoError(t, tx.Rollback())
	assert.Equal(t, 0, countMembers(t, db))
}

// TestNativeExecutorReadsCatalog covers the catalog reads that used to require
// *sql.Row.
func TestNativeExecutorReadsCatalog(t *testing.T) {
	db := memberDB(t, "native_catalog_test")

	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	exec := &nativeExecutor{tx: tx}
	dbc, err := NewDBConnectorFromExecutor(exec, SQLiteDialect, true)
	assert.NoError(t, err)

	tables, err := dbc.TableNames(t.Context())
	assert.NoError(t, err)
	assert.True(t, len(tables) > 0)

	pkeys, err := dbc.PrimaryKeys(t.Context(), "member")
	assert.NoError(t, err)
	assert.Equal(t, []string{"id"}, pkeys)

	assert.Equal(t, 0, exec.openCursors(), "catalog reads must close their cursors")
}
