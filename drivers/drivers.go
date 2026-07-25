// Package drivers registers the database/sql drivers that
// dbtestify.NewDBConnector can open.
//
// The core dbtestify package deliberately imports no driver so that consumers
// which already own a *sql.DB can use dbtestify.NewDBConnectorFromDB without
// pulling in cgo-based SQLite, PostgreSQL, and MySQL dependencies.
//
// Import this package for its side effects when you want dbtestify to open the
// connection for you:
//
//	import (
//		"github.com/shibukawa/dbtestify"
//		_ "github.com/shibukawa/dbtestify/drivers"
//	)
//
//	dbc, err := dbtestify.NewDBConnector(ctx, "sqlite://file:test.db")
package drivers

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)
