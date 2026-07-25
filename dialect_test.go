package dbtestify

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestParseDialect(t *testing.T) {
	tests := []struct {
		name    string
		want    Dialect
		wantErr bool
	}{
		{name: "postgres", want: PostgresDialect},
		{name: "postgresql", want: PostgresDialect},
		{name: "pgx", want: PostgresDialect},
		{name: "mysql", want: MySQLDialect},
		{name: "MySQL", want: MySQLDialect},
		{name: "sqlite", want: SQLiteDialect},
		{name: "sqlite3", want: SQLiteDialect},
		{name: " sqlite ", want: SQLiteDialect},
		{name: "oracle", wantErr: true},
		{name: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseDialect(test.name)
			if test.wantErr {
				assert.IsError(t, err, ErrInvalidDialect)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
