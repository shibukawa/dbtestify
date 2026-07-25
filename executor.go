package dbtestify

import (
	"context"
	"database/sql"
)

// Executor is the subset of database/sql that dbtestify runs statements
// through. Both *sql.DB and *sql.Tx satisfy it, so a DBConnector can work on a
// pool or inside a transaction the caller already opened.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var (
	_ Executor = (*sql.DB)(nil)
	_ Executor = (*sql.Tx)(nil)
	_ Executor = (*sql.Conn)(nil)
)

// baseConnector holds the execution target shared by every dialect connector.
type baseConnector struct {
	exec Executor
	// db is nil when the connector wraps a transaction or a bare Executor.
	db   *sql.DB
	inTx bool
}

// Executor returns the target every statement runs on.
func (base *baseConnector) Executor() Executor {
	return base.exec
}

// DB returns the pool when the connector owns one, and nil when it wraps a
// transaction or a bare Executor.
func (base *baseConnector) DB() *sql.DB {
	return base.db
}

// InTransaction reports whether Executor is already inside a transaction, in
// which case [Seed] must not open or commit one of its own.
func (base *baseConnector) InTransaction() bool {
	return base.inTx
}
