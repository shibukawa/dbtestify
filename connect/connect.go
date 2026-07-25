// Package connect opens a dbtestify.DBConnector from a connection string.
//
// It is separate from the core dbtestify package because it registers the
// SQLite, PostgreSQL, and MySQL database/sql drivers. Consumers that already
// own a *sql.DB use dbtestify.NewDBConnectorFromDB instead and never pay for
// those dependencies, which keeps the core package free of cgo.
package connect

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"

	"github.com/shibukawa/dbtestify"
)

// driverNames maps a dialect to the database/sql driver this package registers.
var driverNames = map[dbtestify.Dialect]string{
	dbtestify.PostgresDialect: "pgx",
	dbtestify.MySQLDialect:    "mysql",
	dbtestify.SQLiteDialect:   "sqlite3",
}

// NewDBConnector creates a new dbtestify.DBConnector based on the provided
// source string. The source string should be in the format of "mysql://",
// "sqlite://", or "postgres://".
//
// The context is used to manage the lifecycle of the database connection. So you should pass a context that can be cancelled.
// The returned DBConnector owns the *sql.DB it opened and closes it when ctx is done.
// Use dbtestify.NewDBConnectorFromDB instead when the caller already owns the pool.
//
// This package uses https://github.com/jackc/pgx for PostgreSQL, https://github.com/go-sql-driver/mysql for MySQL, https://github.com/mattn/go-sqlite3 for SQLite3.
// To study the detail of the connection string, check the documentation of each driver.
//
//   - postgres://user:pass@localhost:5432/dbname?sslmode=disable
//   - mysql://root:pass@tcp(localhost:3306)/foo?tls=skip-verify
//   - sqlite://file:dbfilename.db
func NewDBConnector(ctx context.Context, source string) (dbtestify.DBConnector, error) {
	dialect, dsn, err := dbtestify.SplitSource(source)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid driver '%s'", dbtestify.ErrInvalidDBDriver, source)
	}
	driver, ok := driverNames[dialect]
	if !ok {
		return nil, fmt.Errorf("%w: invalid driver '%s'", dbtestify.ErrInvalidDBDriver, source)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: can't connect to %s '%s': %s", dbtestify.ErrInvalidDBDriver, dialect, err.Error(), dsn)
	}
	go func() {
		<-ctx.Done()
		db.Close()
	}()
	connector, err := dbtestify.NewDBConnectorFromDB(db, dialect)
	if err != nil {
		db.Close()
		return nil, err
	}
	return connector, nil
}
