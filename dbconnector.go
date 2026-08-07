package dbtestify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

var ErrInvalidDBDriver = errors.New("invalid db driver")

// DBConnector is an interface for database operations that is used from dbtestify package.
// It absorbs the database driver specific operations like getting table names, primary keys, and performing CRUD operations.
type DBConnector interface {
	TableNames(ctx context.Context, schema ...string) ([]string, error)
	PrimaryKeys(ctx context.Context, table string) ([]string, error)
	Insert(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error)
	Delete(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error)
	Upsert(ctx context.Context, exec Executor, tableName string, columns, pKeys []string, values []any) (int64, error)
	Truncate(ctx context.Context, exec Executor, tableName string) (int64, error)

	// Executor returns the target statements run on.
	Executor() Executor
	// DB returns the pool when the connector owns one, and nil when it wraps a
	// transaction.
	DB() *sql.DB
	// InTransaction reports whether Executor is already inside a transaction.
	InTransaction() bool
}

// NewDBConnectorFromDB creates a DBConnector on top of a *sql.DB the caller
// already owns.
//
// dbtestify never closes db and never changes its pool settings, so the caller
// stays responsible for its lifecycle. Use this when the pool is owned by an
// application framework, or when the database is unreachable from a second
// pool such as an in-memory SQLite database.
//
// dialect selects the SQL flavor because a *sql.DB does not expose the name it
// was opened with. Use [ParseDialect] or [SplitSource] to derive it from a DSN.
//
// This package registers no database/sql driver. To open a connection from a
// connection string instead, use github.com/shibukawa/dbtestify/connect.
func NewDBConnectorFromDB(db *sql.DB, dialect Dialect) (DBConnector, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: nil *sql.DB", ErrInvalidDBDriver)
	}
	return newConnector(baseConnector{exec: FromSQL(db), db: db}, dialect)
}

// NewDBConnectorFromTx creates a DBConnector that runs inside a transaction the
// caller already opened.
//
// [Seed] then opens no transaction of its own and commits nothing, so the
// caller decides whether the seeded rows are committed or rolled back. [Assert]
// sees the transaction's uncommitted state, which a pool-based connector cannot
// observe.
//
// dbtestify never commits or rolls back tx.
func NewDBConnectorFromTx(tx *sql.Tx, dialect Dialect) (DBConnector, error) {
	if tx == nil {
		return nil, fmt.Errorf("%w: nil *sql.Tx", ErrInvalidDBDriver)
	}
	return newConnector(baseConnector{exec: FromSQL(tx), inTx: true}, dialect)
}

// NewDBConnectorFromExecutor creates a DBConnector on top of any [Executor],
// including one backed by a driver that bypasses database/sql. Wrap a
// database/sql handle such as a session-pinned *sql.Conn with [FromSQL].
//
// Set inTransaction when the executor is already inside a transaction so [Seed]
// does not try to open another one. A *sql.Conn is not in a transaction unless
// the caller started one on it.
//
// The connector owns no pool, so [Seed] on one built here requires
// inTransaction. Use [NewDBConnectorFromDB] to let Seed manage its own
// transaction.
func NewDBConnectorFromExecutor(exec Executor, dialect Dialect, inTransaction bool) (DBConnector, error) {
	if exec == nil {
		return nil, fmt.Errorf("%w: nil Executor", ErrInvalidDBDriver)
	}
	return newConnector(baseConnector{exec: exec, inTx: inTransaction}, dialect)
}

func newConnector(base baseConnector, dialect Dialect) (DBConnector, error) {
	switch dialect {
	case PostgresDialect:
		return &psqlDBConnector{baseConnector: base}, nil
	case MySQLDialect:
		return &mysqlDBConnector{baseConnector: base}, nil
	case SQLiteDialect:
		return &sqliteDBConnector{baseConnector: base}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidDialect, string(dialect))
	}
}

type psqlDBConnector struct {
	baseConnector
}

func (p *psqlDBConnector) TableNames(ctx context.Context, schema ...string) ([]string, error) {
	var s string
	if len(schema) == 0 {
		var err error
		s, err = queryString(ctx, p.exec, `SELECT current_schema();`)
		if err != nil {
			return nil, err
		}
	} else {
		s = schema[0]
	}
	return queryStrings(ctx, p.exec, `
		SELECT
			tablename
		FROM
			pg_catalog.pg_tables
		WHERE
			schemaname = $1
		ORDER BY
			tablename;`, s)
}

func (p *psqlDBConnector) PrimaryKeys(ctx context.Context, table string) ([]string, error) {
	var schema, tname string
	f := strings.SplitN(table, ".", 2)
	if len(f) == 2 {
		schema = f[0]
		tname = f[1]
	} else {
		var err error
		schema, err = queryString(ctx, p.exec, `SELECT current_schema();`)
		if err != nil {
			return nil, err
		}
		tname = table
	}
	return queryStrings(ctx, p.exec, `
		SELECT
			kcu.column_name
		FROM
			information_schema.table_constraints AS tc
		JOIN
			information_schema.key_column_usage AS kcu
		ON
			tc.constraint_name = kcu.constraint_name
		AND
			tc.table_schema = kcu.table_schema
		WHERE
			tc.constraint_type = 'PRIMARY KEY'
		AND
			tc.table_schema = $1
		AND
			tc.table_name = $2
		ORDER BY
			kcu.column_name;
	`, schema, tname)
}

func pgPlaceholders(columns, rows int) string {
	c := 1
	var placeholders []string
	for r := 0; r < rows; r++ {
		var rp []string
		for i := 0; i < columns; i++ {
			rp = append(rp, "$"+strconv.Itoa(c))
			c++
		}
		placeholders = append(placeholders, "("+strings.Join(rp, ", ")+")")
	}
	return strings.Join(placeholders, ", ")
}

func (p *psqlDBConnector) Insert(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error) {
	insertStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		tableName,
		strings.Join(columns, ", "),
		pgPlaceholders(len(columns), len(values)/len(columns)),
	)
	return exec.Exec(ctx, insertStmt, values...)
}

func (p *psqlDBConnector) Delete(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error) {
	var columnStr string
	var placeholderStr string
	if len(columns) > 1 {
		placeholderStr = pgPlaceholders(len(columns), len(values)/len(columns))
	} else {
		columnStr = columns[0]
		var placeholders []string
		for i := range len(columns) {
			placeholders = append(placeholders, "$"+strconv.Itoa(i+1))
		}
		placeholderStr = strings.Join(placeholders, ", ")
	}

	deleteStmt := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN (%s)",
		tableName,
		columnStr,
		placeholderStr,
	)
	return exec.Exec(ctx, deleteStmt, values...)
}

// Truncate implements DBConnector.
func (p *psqlDBConnector) Truncate(ctx context.Context, exec Executor, tableName string) (int64, error) {
	return exec.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s;", tableName))
}

// Upsert implements DBConnector.
func (p *psqlDBConnector) Upsert(ctx context.Context, exec Executor, tableName string, columns, pKeys []string, values []any) (int64, error) {
	var assigns []string
	for _, column := range columns {
		if !slices.Contains(pKeys, column) {
			assigns = append(assigns, column+" = excluded."+column)
		}
	}

	insertStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s ON CONFLICT(%s) DO UPDATE SET %s;",
		tableName,
		strings.Join(columns, ", "),
		pgPlaceholders(len(columns), len(values)/len(columns)),
		strings.Join(pKeys, ", "),
		strings.Join(assigns, ", "),
	)
	return exec.Exec(ctx, insertStmt, values...)
}

var _ DBConnector = (*psqlDBConnector)(nil)

type mysqlDBConnector struct {
	baseConnector
}

func (m *mysqlDBConnector) TableNames(ctx context.Context, schema ...string) ([]string, error) {
	var s string
	if len(schema) == 0 {
		var err error
		s, err = queryString(ctx, m.exec, `SELECT DATABASE();`)
		if err != nil {
			return nil, err
		}
	} else {
		s = schema[0]
	}
	return queryStrings(ctx, m.exec, `
		SELECT
			t.table_name
		FROM
			information_schema.tables AS t
		WHERE
			t.table_schema = ?
		AND
			t.table_type = 'BASE TABLE'
		ORDER BY
			t.table_name;
	`, s)
}

func (m *mysqlDBConnector) PrimaryKeys(ctx context.Context, table string) ([]string, error) {
	var schema, tname string
	f := strings.SplitN(table, ".", 2)
	if len(f) == 2 {
		schema = f[0]
		tname = f[1]
	} else {
		var err error
		schema, err = queryString(ctx, m.exec, `SELECT DATABASE();`)
		if err != nil {
			return nil, err
		}
		tname = table
	}
	return queryStrings(ctx, m.exec, `
		SELECT
			c.COLUMN_NAME
		FROM
			information_schema.COLUMNS AS c
		WHERE
			c.TABLE_SCHEMA = ?
		AND
			c.TABLE_NAME = ?
		AND
			c.COLUMN_KEY = 'PRI'
		ORDER BY
			c.COLUMN_NAME;
	`, schema, tname)
}

func (m *mysqlDBConnector) Insert(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error) {
	insertStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s;",
		tableName,
		strings.Join(columns, ", "),
		slPlaceholders(len(columns), len(values)/len(columns)),
	)
	return exec.Exec(ctx, insertStmt, values...)
}

func (m *mysqlDBConnector) Delete(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error) {
	var columnStr string
	var placeholderStr string
	if len(columns) > 1 {
		placeholderStr = slPlaceholders(len(columns), len(values)/len(columns))
	} else {
		columnStr = columns[0]
		placeholderStr = strings.Join(slices.Repeat([]string{"?"}, len(values)), ", ")
	}

	deleteStmt := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN (%s);",
		tableName,
		columnStr,
		placeholderStr,
	)

	return exec.Exec(ctx, deleteStmt, values...)
}

func (m *mysqlDBConnector) Upsert(ctx context.Context, exec Executor, tableName string, columns, pKeys []string, values []any) (int64, error) {
	var assigns []string
	for _, column := range columns {
		if !slices.Contains(pKeys, column) {
			assigns = append(assigns, column+" = VALUES("+column+")")
		}
	}

	insertStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s ON DUPLICATE KEY UPDATE %s;",
		tableName,
		strings.Join(columns, ", "),
		slPlaceholders(len(columns), len(values)/len(columns)),
		strings.Join(assigns, ", "),
	)
	return exec.Exec(ctx, insertStmt, values...)
}

func (m *mysqlDBConnector) Truncate(ctx context.Context, exec Executor, tableName string) (int64, error) {
	return exec.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s;", tableName))
}

var _ DBConnector = (*mysqlDBConnector)(nil)

type sqliteDBConnector struct {
	baseConnector
}

func (s *sqliteDBConnector) TableNames(ctx context.Context, schema ...string) ([]string, error) {
	return queryStrings(ctx, s.exec, `
		SELECT
			sm.name
		FROM
			sqlite_master AS sm
		WHERE
			sm.type='table'
		ORDER BY
			sm.name;`)
}

func (s *sqliteDBConnector) PrimaryKeys(ctx context.Context, table string) ([]string, error) {
	return queryStrings(ctx, s.exec, `
		SELECT
			ti.name
		FROM
			pragma_table_info(?) AS ti
		WHERE
			ti.pk <> 0
		ORDER BY
			ti.name;`, table)
}

func slPlaceholders(columns, records int) string {
	rp := "(" + strings.Join(slices.Repeat([]string{"?"}, columns), ", ") + ")"
	placeholders := slices.Repeat([]string{rp}, records)
	return strings.Join(placeholders, ", ")
}

func (s *sqliteDBConnector) Insert(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error) {
	insertStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		tableName,
		strings.Join(columns, ", "),
		slPlaceholders(len(columns), len(values)/len(columns)),
	)
	return exec.Exec(ctx, insertStmt, values...)
}

func (s *sqliteDBConnector) Delete(ctx context.Context, exec Executor, tableName string, columns []string, values []any) (int64, error) {
	var columnStr string
	var placeholderStr string
	if len(columns) > 1 {
		placeholderStr = slPlaceholders(len(columns), len(values)/len(columns))
	} else {
		columnStr = columns[0]
		placeholderStr = strings.Join(slices.Repeat([]string{"?"}, len(values)), ", ")
	}

	deleteStmt := fmt.Sprintf(
		"DELETE FROM %s WHERE %s IN (%s)",
		tableName,
		columnStr,
		placeholderStr,
	)
	return exec.Exec(ctx, deleteStmt, values...)
}

func (s *sqliteDBConnector) Upsert(ctx context.Context, exec Executor, tableName string, columns, pKeys []string, values []any) (int64, error) {
	var assigns []string
	for _, column := range columns {
		if !slices.Contains(pKeys, column) {
			assigns = append(assigns, column+" = excluded."+column)
		}
	}

	insertStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s ON CONFLICT(%s) DO UPDATE SET %s;",
		tableName,
		strings.Join(columns, ", "),
		slPlaceholders(len(columns), len(values)/len(columns)),
		strings.Join(pKeys, ", "),
		strings.Join(assigns, ", "),
	)
	return exec.Exec(ctx, insertStmt, values...)
}

func (s *sqliteDBConnector) Truncate(ctx context.Context, exec Executor, tableName string) (int64, error) {
	truncateSrc := fmt.Sprintf("DELETE FROM %s;", tableName)
	return exec.Exec(ctx, truncateSrc)
}

var _ DBConnector = (*sqliteDBConnector)(nil)
