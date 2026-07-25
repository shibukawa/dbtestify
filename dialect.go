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

// SplitSource splits a "scheme://dsn" connection string into its Dialect and
// the DSN to hand to sql.Open.
//
// PostgreSQL DSNs keep their scheme because pgx expects a full URL; the other
// dialects drop it.
func SplitSource(source string) (Dialect, string, error) {
	scheme, dsn, ok := strings.Cut(source, "://")
	if !ok {
		return "", "", fmt.Errorf("%w: missing scheme in %q", ErrInvalidDialect, source)
	}
	dialect, err := ParseDialect(scheme)
	if err != nil {
		return "", "", err
	}
	if dialect == PostgresDialect {
		return dialect, source, nil
	}
	return dialect, dsn, nil
}
