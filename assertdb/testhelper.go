// assertdb is a helper for testing package. It seeds/asserts the database with the data from the specified YAML file.
//
// It assumes the yaml is in embed.FS
//
//	//go:embed dataset/*
//	var dataSet embed.FS
//
//	func TestUsage(t *testing.T) {
//	    assertdb.SeedDataSet(t, "sqlite://file:database.db", dataSet, "initial.yaml", nil)
//
//	    // some logic that modifies the database
//
//	    assertdb.AssertDB(t, "sqlite://file:database.db", dataSet, "expect.yaml", nil)
//	}
//
// SeedDataSetDB and AssertDBWithDB accept a *sql.DB the caller already owns.
// Use them when a framework owns the pool, or when the database is unreachable
// from a second pool such as an in-memory SQLite database.
package assertdb

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"

	"github.com/shibukawa/dbtestify"
	"github.com/shibukawa/dbtestify/connect"
)

// SeedDataSet seeds the database with the data from the specified YAML file.
func SeedDataSet(t *testing.T, dbConn string, folder fs.FS, fileName string, opt *dbtestify.SeedOpt) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dbc, err := connect.NewDBConnector(ctx, dbConn)
	if err != nil {
		t.Fatalf("Failed to create DB connector: %v", err)
		return
	}
	seed(t, ctx, dbc, folder, fileName, opt)
}

// SeedDataSetDB seeds a database the caller already opened.
//
// dialect selects the SQL flavor; use dbtestify.ParseDialect to derive it from
// a DSN scheme. The pool is never closed by this helper.
func SeedDataSetDB(t *testing.T, db *sql.DB, dialect dbtestify.Dialect, folder fs.FS, fileName string, opt *dbtestify.SeedOpt) {
	t.Helper()
	dbc, err := dbtestify.NewDBConnectorFromDB(db, dialect)
	if err != nil {
		t.Fatalf("Failed to create DB connector: %v", err)
		return
	}
	seed(t, t.Context(), dbc, folder, fileName, opt)
}

// AssertDB asserts the database state against the data from the specified YAML file.
func AssertDB(t *testing.T, dbConn string, folder fs.FS, fileName string, opt *dbtestify.AssertOpt) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dbc, err := connect.NewDBConnector(ctx, dbConn)
	if err != nil {
		t.Fatalf("Failed to create DB connector: %v", err)
		return
	}
	assertDB(t, ctx, dbc, folder, fileName, opt)
}

// AssertDBWithDB asserts the state of a database the caller already opened.
//
// dialect selects the SQL flavor; use dbtestify.ParseDialect to derive it from
// a DSN scheme. The pool is never closed by this helper.
func AssertDBWithDB(t *testing.T, db *sql.DB, dialect dbtestify.Dialect, folder fs.FS, fileName string, opt *dbtestify.AssertOpt) {
	t.Helper()
	dbc, err := dbtestify.NewDBConnectorFromDB(db, dialect)
	if err != nil {
		t.Fatalf("Failed to create DB connector: %v", err)
		return
	}
	assertDB(t, t.Context(), dbc, folder, fileName, opt)
}

// SeedDataSetTx seeds inside a transaction the caller already opened.
//
// Nothing is committed or rolled back by this helper, so a test that rolls the
// transaction back leaves no trace of the dataset.
func SeedDataSetTx(t *testing.T, tx *sql.Tx, dialect dbtestify.Dialect, folder fs.FS, fileName string, opt *dbtestify.SeedOpt) {
	t.Helper()
	dbc, err := dbtestify.NewDBConnectorFromTx(tx, dialect)
	if err != nil {
		t.Fatalf("Failed to create DB connector: %v", err)
		return
	}
	seed(t, t.Context(), dbc, folder, fileName, opt)
}

// AssertDBTx asserts the state seen inside a transaction the caller already
// opened, including writes that transaction has not committed.
func AssertDBTx(t *testing.T, tx *sql.Tx, dialect dbtestify.Dialect, folder fs.FS, fileName string, opt *dbtestify.AssertOpt) {
	t.Helper()
	dbc, err := dbtestify.NewDBConnectorFromTx(tx, dialect)
	if err != nil {
		t.Fatalf("Failed to create DB connector: %v", err)
		return
	}
	assertDB(t, t.Context(), dbc, folder, fileName, opt)
}

func parseDataSet(t *testing.T, folder fs.FS, fileName string) *dbtestify.DataSet {
	t.Helper()
	file, err := folder.Open(fileName)
	if err != nil {
		t.Fatalf("Failed to open dataset %s: %v", fileName, err)
		return nil
	}
	defer file.Close()
	data, err := dbtestify.ParseYAML(file)
	if err != nil {
		t.Fatalf("Failed to parse dataset %s: %v", fileName, err)
		return nil
	}
	return data
}

func seed(t *testing.T, ctx context.Context, dbc dbtestify.DBConnector, folder fs.FS, fileName string, opt *dbtestify.SeedOpt) {
	t.Helper()
	data := parseDataSet(t, folder, fileName)
	if data == nil {
		return
	}
	if opt == nil {
		opt = &dbtestify.SeedOpt{}
	}
	if err := dbtestify.Seed(ctx, dbc, data, *opt); err != nil {
		t.Fatalf("Failed to seed dataset %s: %v", fileName, err)
	}
}

func assertDB(t *testing.T, ctx context.Context, dbc dbtestify.DBConnector, folder fs.FS, fileName string, opt *dbtestify.AssertOpt) {
	t.Helper()
	data := parseDataSet(t, folder, fileName)
	if data == nil {
		return
	}
	if opt == nil {
		opt = &dbtestify.AssertOpt{}
	}
	var diff strings.Builder
	opt.DiffCallback = dbtestify.DumpDiffCallback(&diff, dbtestify.DiffFormat{
		ShowTableName: true,
		Quiet:         true,
		NoColor:       true,
	})
	ok, _, err := dbtestify.Assert(ctx, dbc, data, *opt)
	if err != nil {
		t.Fatalf("Failed to assert dataset %s: %v", fileName, err)
		return
	}
	if !ok {
		t.Errorf("Assertion failed for dataset %s\n%s", fileName, diff.String())
	}
}
