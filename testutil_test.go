package dbtestify

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

// testDriverNames mirrors the mapping in the connect package. These tests live
// in package dbtestify, so they cannot import connect without an import cycle.
var testDriverNames = map[Dialect]string{
	PostgresDialect: "pgx",
	MySQLDialect:    "mysql",
	SQLiteDialect:   "sqlite3",
}

// openTestDBConnector is the in-package equivalent of connect.NewDBConnector.
func openTestDBConnector(ctx context.Context, source string) (DBConnector, error) {
	dialect, dsn, err := SplitSource(source)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(testDriverNames[dialect], dsn)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		db.Close()
	}()
	return NewDBConnectorFromDB(db, dialect)
}

var stripSpacePattern = regexp.MustCompile("(^[ \t]*)")

func TrimIndent(t *testing.T, src string, replace ...string) string {
	t.Helper()
	lines := strings.Split(src, "\n")
	if lines[0] == "" {
		lines = lines[1:]
	}
	matches := stripSpacePattern.FindStringSubmatch(lines[0])
	var b strings.Builder
	for i, line := range lines {
		b.WriteString(strings.TrimPrefix(line, matches[0]))
		if i != len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	result := strings.TrimRight(b.String(), "\n")
	if len(replace) > 1 {
		result = strings.ReplaceAll(result, replace[0], replace[1])
	}
	return result
}
