package serve

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"

	"github.com/shibukawa/dbtestify/httpapi"
)

func newTestDir(t *testing.T) (dir, dbconn string) {
	t.Helper()
	dir = t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "users.yaml"), []byte(`
member:
- { id: 1, name: Frank, email: frank@example.com }
`), 0o600)
	assert.NoError(t, err)

	dbFile := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite3", dbFile)
	assert.NoError(t, err)
	defer db.Close()
	_, err = db.ExecContext(t.Context(), `
		CREATE TABLE member (
			id INTEGER NOT NULL,
			name TEXT NOT NULL,
			email TEXT,
			PRIMARY KEY (id)
		);
	`)
	assert.NoError(t, err)
	return dir, "sqlite://file:" + dbFile
}

// TestNewConfigReusesOneConnector is the fix for a pool and a goroutine leaking
// on every request: the server connects once and hands the same connector out
// for the rest of its life.
func TestNewConfigReusesOneConnector(t *testing.T) {
	dir, dbconn := newTestDir(t)

	cfg, err := newConfig(t.Context(), dir, dbconn)
	assert.NoError(t, err)

	first, err := cfg.Connector(t.Context())
	assert.NoError(t, err)
	second, err := cfg.Connector(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, first.DB(), second.DB())
}

// TestServeBehavesLikeBefore drives the handler the CLI serves, over the paths
// a pre-v0.4.0 harness uses.
func TestServeBehavesLikeBefore(t *testing.T) {
	dir, dbconn := newTestDir(t)

	cfg, err := newConfig(t.Context(), dir, dbconn)
	assert.NoError(t, err)
	h, err := httpapi.NewHandler(cfg)
	assert.NoError(t, err)
	s := httptest.NewServer(h)
	defer s.Close()

	res, err := http.Post(s.URL+"/api/seed/users.yaml", "", nil)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	res, err = http.Get(s.URL + "/api/assert/users.yaml")
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	res, err = http.Get(s.URL + "/api/list")
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestNewConfigRejectsBadInput(t *testing.T) {
	dir, dbconn := newTestDir(t)

	_, err := newConfig(t.Context(), filepath.Join(dir, "nope"), dbconn)
	assert.Error(t, err)

	_, err = newConfig(t.Context(), t.TempDir(), dbconn)
	assert.Error(t, err, "an empty data set directory should be reported before the listener opens")

	_, err = newConfig(t.Context(), dir, "sqlite://file:"+filepath.Join(t.TempDir(), "missing.db")+"?mode=ro")
	assert.Error(t, err, "an unreachable database should be reported before the listener opens")
}
