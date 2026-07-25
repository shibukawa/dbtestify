package dbtestify

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidDialect is returned when a dialect name is not supported.
var ErrInvalidDialect = errors.New("invalid dialect")

// Dialect identifies the SQL flavor a DBConnector speaks.
//
// It is independent from the database/sql driver name so callers that already
// own a *sql.DB can select the connector behavior explicitly.
type Dialect string

const (
	PostgresDialect Dialect = "postgres"
	MySQLDialect    Dialect = "mysql"
	SQLiteDialect   Dialect = "sqlite"
)

func (d Dialect) String() string {
	return string(d)
}

// ParseDialect resolves a connection string scheme or a database/sql driver
// name to a Dialect.
//
// Accepted names are postgres, postgresql, pgx, mysql, sqlite, and sqlite3.
func ParseDialect(name string) (Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "postgres", "postgresql", "pgx":
		return PostgresDialect, nil
	case "mysql":
		return MySQLDialect, nil
	case "sqlite", "sqlite3":
		return SQLiteDialect, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidDialect, name)
	}
}

// driverName returns the database/sql driver registered by the drivers package
// for this dialect.
func (d Dialect) driverName() (string, error) {
	switch d {
	case PostgresDialect:
		return "pgx", nil
	case MySQLDialect:
		return "mysql", nil
	case SQLiteDialect:
		return "sqlite3", nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidDialect, string(d))
	}
}
