package dbtestify

import (
	"context"
	"database/sql"
)

// Rows is a forward cursor over a result set.
//
// It names no database/sql type, so a backend built on a native driver can
// return its own cursor. *sql.Rows satisfies it unchanged.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
	Columns() ([]string, error)
}

// Executor is the target dbtestify runs every statement through.
//
// Nothing in this interface comes from database/sql, so a backend that
// bypasses the package entirely — a pgx pool, for example — implements it
// directly. Wrap a database/sql handle with [FromSQL].
type Executor interface {
	// Exec runs a statement. dbtestify never reads an affected-row count, so
	// an implementation reports only failure.
	Exec(ctx context.Context, query string, args ...any) error
	// Query runs a query and returns its cursor. dbtestify closes every
	// cursor it receives.
	Query(ctx context.Context, query string, args ...any) (Rows, error)
}

// SQLHandle is the database/sql subset [FromSQL] adapts. *sql.DB, *sql.Tx, and
// *sql.Conn satisfy it.
type SQLHandle interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

var (
	_ SQLHandle = (*sql.DB)(nil)
	_ SQLHandle = (*sql.Tx)(nil)
	_ SQLHandle = (*sql.Conn)(nil)
	_ Rows      = (*sql.Rows)(nil)
	_ Executor  = sqlExecutor{}
)

// FromSQL adapts a database/sql handle to [Executor] so it can be passed to
// [NewDBConnectorFromExecutor].
//
// [NewDBConnectorFromDB] and [NewDBConnectorFromTx] wrap their argument
// themselves; callers of those need not use this.
func FromSQL(h SQLHandle) Executor {
	return sqlExecutor{h: h}
}

type sqlExecutor struct {
	h SQLHandle
}

func (e sqlExecutor) Exec(ctx context.Context, query string, args ...any) error {
	_, err := e.h.ExecContext(ctx, query, args...)
	return err
}

func (e sqlExecutor) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := e.h.QueryContext(ctx, query, args...)
	if err != nil {
		// Return an untyped nil so callers can compare the result against nil.
		return nil, err
	}
	return rows, nil
}

// queryStrings runs a query whose rows hold a single string column and drains
// it into a slice.
func queryStrings(ctx context.Context, exec Executor, query string, args ...any) ([]string, error) {
	rows, err := exec.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// queryString runs a query that yields one row holding one string, which is how
// the catalog connectors read the current schema name.
func queryString(ctx context.Context, exec Executor, query string, args ...any) (string, error) {
	rows, err := exec.Query(ctx, query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", sql.ErrNoRows
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		return "", err
	}
	return value, rows.Err()
}

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
