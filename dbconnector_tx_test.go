package dbtestify

import (
	"database/sql"
	"testing"

	"github.com/alecthomas/assert/v2"
	_ "github.com/mattn/go-sqlite3"
)

func memberDataSet(names ...string) *DataSet {
	table := &Table{Name: "member"}
	for index, name := range names {
		table.Rows = append(table.Rows, map[string]any{"id": index + 1, "name": name})
		table.Tags = append(table.Tags, nil)
	}
	return &DataSet{Tables: []*Table{table}}
}

func memberDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+name+"?mode=memory&cache=shared")
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.ExecContext(t.Context(), "CREATE TABLE member (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
	assert.NoError(t, err)
	return db
}

func countMembers(t *testing.T, h SQLHandle) int {
	t.Helper()
	rows, err := h.QueryContext(t.Context(), "SELECT count(*) FROM member")
	assert.NoError(t, err)
	defer rows.Close()
	assert.True(t, rows.Next())
	var count int
	assert.NoError(t, rows.Scan(&count))
	return count
}

// TestSeedFromTxIsRolledBackWithTheCaller proves Seed neither opens nor commits
// a transaction of its own when the connector wraps one.
func TestSeedFromTxIsRolledBackWithTheCaller(t *testing.T) {
	db := memberDB(t, "tx_rollback_test")

	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)

	dbc, err := NewDBConnectorFromTx(tx, SQLiteDialect)
	assert.NoError(t, err)
	assert.True(t, dbc.InTransaction())
	assert.Equal(t, nil, dbc.DB())

	assert.NoError(t, Seed(t.Context(), dbc, memberDataSet("Frank", "Grace"), SeedOpt{}))
	assert.Equal(t, 2, countMembers(t, tx))

	assert.NoError(t, tx.Rollback())
	assert.Equal(t, 0, countMembers(t, db))
}

func TestSeedFromTxSurvivesCommit(t *testing.T) {
	db := memberDB(t, "tx_commit_test")

	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	dbc, err := NewDBConnectorFromTx(tx, SQLiteDialect)
	assert.NoError(t, err)

	assert.NoError(t, Seed(t.Context(), dbc, memberDataSet("Frank"), SeedOpt{}))
	assert.NoError(t, tx.Commit())

	assert.Equal(t, 1, countMembers(t, db))
}

// TestAssertFromTxSeesUncommittedWrites is the whole point of the transaction
// connector: a pool-based connector cannot observe them.
func TestAssertFromTxSeesUncommittedWrites(t *testing.T) {
	db := memberDB(t, "tx_assert_test")

	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	_, err = tx.ExecContext(t.Context(), "INSERT INTO member VALUES (1, 'Frank')")
	assert.NoError(t, err)

	txConnector, err := NewDBConnectorFromTx(tx, SQLiteDialect)
	assert.NoError(t, err)
	ok, _, err := Assert(t.Context(), txConnector, memberDataSet("Frank"), AssertOpt{})
	assert.NoError(t, err)
	assert.True(t, ok, "transaction connector must see the uncommitted row")

	// The row was never committed, so the pool never had it. Asserting through
	// the pool while the write transaction is open would block on SQLite's
	// table lock rather than report a mismatch, so check after the rollback.
	assert.NoError(t, tx.Rollback())
	assert.Equal(t, 0, countMembers(t, db))
}

func TestNewDBConnectorFromExecutor(t *testing.T) {
	db := memberDB(t, "executor_test")

	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	dbc, err := NewDBConnectorFromExecutor(FromSQL(tx), SQLiteDialect, true)
	assert.NoError(t, err)
	assert.True(t, dbc.InTransaction())
	// A bare Executor never carries a pool, so Seed runs in the caller's
	// transaction rather than opening one.
	assert.Equal(t, nil, dbc.DB())
	assert.NoError(t, Seed(t.Context(), dbc, memberDataSet("Frank"), SeedOpt{}))
	assert.Equal(t, 1, countMembers(t, tx))
}

func TestTransactionConnectorErrors(t *testing.T) {
	_, err := NewDBConnectorFromTx(nil, SQLiteDialect)
	assert.IsError(t, err, ErrInvalidDBDriver)

	_, err = NewDBConnectorFromExecutor(nil, SQLiteDialect, false)
	assert.IsError(t, err, ErrInvalidDBDriver)

	db := memberDB(t, "tx_error_test")
	tx, err := db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	_, err = NewDBConnectorFromTx(tx, Dialect("oracle"))
	assert.IsError(t, err, ErrInvalidDialect)
}

// TestSeedRejectsPoollessConnectorOutsideTransaction covers the one combination
// that cannot work: a bare Executor that is not a pool and not in a transaction.
func TestSeedRejectsPoollessConnectorOutsideTransaction(t *testing.T) {
	db := memberDB(t, "poolless_test")
	conn, err := db.Conn(t.Context())
	assert.NoError(t, err)
	defer conn.Close()

	dbc, err := NewDBConnectorFromExecutor(FromSQL(conn), SQLiteDialect, false)
	assert.NoError(t, err)

	err = Seed(t.Context(), dbc, memberDataSet("Frank"), SeedOpt{})
	assert.Error(t, err)
}
