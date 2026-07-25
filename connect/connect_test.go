package connect_test

import (
	"os"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/shibukawa/dbtestify"
	"github.com/shibukawa/dbtestify/connect"
)

func TestNewDBConnectorSQLite(t *testing.T) {
	t.Cleanup(func() { os.Remove("connect_test.db") })

	dbc, err := connect.NewDBConnector(t.Context(), "sqlite://file:connect_test.db?cache=shared&mode=rwc")
	assert.NoError(t, err)

	_, err = dbc.DB().ExecContext(t.Context(), "CREATE TABLE IF NOT EXISTS member (id INTEGER PRIMARY KEY, name TEXT)")
	assert.NoError(t, err)

	pkeys, err := dbc.PrimaryKeys(t.Context(), "member")
	assert.NoError(t, err)
	assert.Equal(t, []string{"id"}, pkeys)
}

func TestNewDBConnectorRejectsUnknownScheme(t *testing.T) {
	_, err := connect.NewDBConnector(t.Context(), "oracle://localhost/orcl")
	assert.IsError(t, err, dbtestify.ErrInvalidDBDriver)

	_, err = connect.NewDBConnector(t.Context(), "file:no-scheme.db")
	assert.IsError(t, err, dbtestify.ErrInvalidDBDriver)
}
