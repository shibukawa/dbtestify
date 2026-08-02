package httpapi_test

import (
	"context"
	"database/sql"
	"net/http"
	"os"

	"github.com/shibukawa/dbtestify"
	"github.com/shibukawa/dbtestify/httpapi"
)

// ExampleNewHandler mounts the API on the application's own listener, using the
// pool the application already opened.
func ExampleNewHandler() {
	var pool *sql.DB // the pool the application under test uses

	h, err := httpapi.NewHandler(httpapi.Config{
		Connector: func(ctx context.Context) (dbtestify.DBConnector, error) {
			return dbtestify.NewDBConnectorFromDB(pool, dbtestify.SQLiteDialect)
		},
		DataSets:   os.DirFS("testdata"),
		AlwaysJSON: true,
	})
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/_e2e/api/", http.StripPrefix("/_e2e", h))

	// POST /_e2e/api/seed/users.yaml    seeds the data set
	// GET  /_e2e/api/assert/users.yaml  asserts it, and answers 400 on a mismatch
	// GET  /_e2e/api/list               lists the data sets
}

// ExampleNewHandler_transaction scopes each request to a transaction the
// caller owns, so a scenario can be rolled back afterwards.
func ExampleNewHandler_transaction() {
	var tx *sql.Tx // opened by the harness, rolled back when the scenario ends

	h, _ := httpapi.NewHandler(httpapi.Config{
		Connector: func(ctx context.Context) (dbtestify.DBConnector, error) {
			return dbtestify.NewDBConnectorFromTx(tx, dbtestify.SQLiteDialect)
		},
		DataSets: os.DirFS("testdata"),
	})
	_ = h
}
