package dbtestify

import (
	"database/sql"
	"slices"
	"testing"

	"github.com/alecthomas/assert/v2"
	_ "github.com/mattn/go-sqlite3"
)

// TestNewDBConnectorFromDBSharesPool proves that a caller-owned in-memory
// SQLite database is reachable, which is impossible through NewDBConnector
// because that opens a second pool.
func TestNewDBConnectorFromDBSharesPool(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:fromdb_test?mode=memory&cache=shared")
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecContext(t.Context(), TrimIndent(t, `
		CREATE TABLE member (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL
		);
		`))
	assert.NoError(t, err)

	dbc, err := NewDBConnectorFromDB(db, SQLiteDialect)
	assert.NoError(t, err)
	assert.Equal(t, db, dbc.DB())

	tables, err := dbc.TableNames(t.Context())
	assert.NoError(t, err)
	assert.True(t, slices.Contains(tables, "member"), "tables = %v", tables)

	pkeys, err := dbc.PrimaryKeys(t.Context(), "member")
	assert.NoError(t, err)
	assert.Equal(t, []string{"id"}, pkeys)

	data := &DataSet{Tables: []*Table{{
		Name: "member",
		Rows: []map[string]any{{"id": 1, "name": "Frank"}},
		Tags: [][]string{nil},
	}}}
	assert.NoError(t, Seed(t.Context(), dbc, data, SeedOpt{}))

	ok, _, err := Assert(t.Context(), dbc, data, AssertOpt{})
	assert.NoError(t, err)
	assert.True(t, ok)
}

// TestNewDBConnectorFromDBKeepsPoolOpen documents that the connector never
// closes a pool it did not open.
func TestNewDBConnectorFromDBKeepsPoolOpen(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:fromdb_lifecycle_test?mode=memory&cache=shared")
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = NewDBConnectorFromDB(db, SQLiteDialect)
	assert.NoError(t, err)
	assert.NoError(t, db.PingContext(t.Context()))
}

func TestNewDBConnectorFromDBErrors(t *testing.T) {
	_, err := NewDBConnectorFromDB(nil, SQLiteDialect)
	assert.IsError(t, err, ErrInvalidDBDriver)

	db, err := sql.Open("sqlite3", "file:fromdb_error_test?mode=memory&cache=shared")
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = NewDBConnectorFromDB(db, Dialect("oracle"))
	assert.IsError(t, err, ErrInvalidDialect)
}

func TestSplitSource(t *testing.T) {
	dialect, dsn, err := SplitSource("sqlite://file:x.db")
	assert.NoError(t, err)
	assert.Equal(t, SQLiteDialect, dialect)
	assert.Equal(t, "file:x.db", dsn)

	dialect, dsn, err = SplitSource("mysql://root:pass@tcp(localhost:3306)/foo")
	assert.NoError(t, err)
	assert.Equal(t, MySQLDialect, dialect)
	assert.Equal(t, "root:pass@tcp(localhost:3306)/foo", dsn)

	// pgx wants the whole URL, so the scheme survives.
	dialect, dsn, err = SplitSource("postgres://user@localhost:5432/db")
	assert.NoError(t, err)
	assert.Equal(t, PostgresDialect, dialect)
	assert.Equal(t, "postgres://user@localhost:5432/db", dsn)

	_, _, err = SplitSource("oracle://localhost/orcl")
	assert.IsError(t, err, ErrInvalidDialect)

	_, _, err = SplitSource("file:no-scheme.db")
	assert.IsError(t, err, ErrInvalidDialect)
}
