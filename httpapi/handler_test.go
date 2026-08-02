package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/alecthomas/assert/v2"
	_ "github.com/mattn/go-sqlite3"

	"github.com/shibukawa/dbtestify"
	"github.com/shibukawa/dbtestify/httpapi"
)

const mountPrefix = "/_e2e"

var dataSets = fstest.MapFS{
	"users.yaml": &fstest.MapFile{Data: []byte(`
member:
- { id: 1, name: Frank, email: frank@example.com, _tag: [admin] }
- { id: 2, name: Grace, email: grace@example.com }
`)},
	"expect-ok.yaml": &fstest.MapFile{Data: []byte(`
member:
- { id: 1, name: Frank, email: frank@example.com }
- { id: 2, name: Grace, email: grace@example.com }
`)},
	"expect-ng.yaml": &fstest.MapFile{Data: []byte(`
member:
- { id: 1, name: Mallory, email: mallory@example.com }
- { id: 2, name: Grace, email: grace@example.com }
`)},
	"nested/expect-admin.yaml": &fstest.MapFile{Data: []byte(`
_match:
  member: sub

member:
- { id: 1, name: Frank, email: frank@example.com }
`)},
}

// countingFS reports how often a dataset was opened, so a test can prove that
// a rejected request never reached the file system.
type countingFS struct {
	fs.FS
	opened int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.opened++
	return c.FS.Open(name)
}

// newTestServer mounts the API under a non-root prefix over an in-memory
// SQLite database the test owns. Such a database is unreachable from a second
// pool, so this also proves the handler uses the caller's connection.
func newTestServer(t *testing.T, cfg httpapi.Config) (*httptest.Server, *sql.DB) {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+t.Name()+"?mode=memory&cache=shared")
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	// One connection keeps the shared in-memory database alive for the test.
	db.SetMaxIdleConns(1)

	_, err = db.ExecContext(t.Context(), `
		CREATE TABLE member (
			id INTEGER NOT NULL,
			name TEXT NOT NULL,
			email TEXT,
			PRIMARY KEY (id)
		);
	`)
	assert.NoError(t, err)

	if cfg.Connector == nil {
		cfg.Connector = func(context.Context) (dbtestify.DBConnector, error) {
			return dbtestify.NewDBConnectorFromDB(db, dbtestify.SQLiteDialect)
		}
	}
	if cfg.DataSets == nil {
		cfg.DataSets = dataSets
	}
	h, err := httpapi.NewHandler(cfg)
	assert.NoError(t, err)

	mux := http.NewServeMux()
	mux.Handle(mountPrefix+"/api/", http.StripPrefix(mountPrefix, h))
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s, db
}

func request(t *testing.T, method, url string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, body)
	assert.NoError(t, err)
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	var result T
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	return result
}

func TestHandlerSeedAndAssert(t *testing.T) {
	s, db := newTestServer(t, httpapi.Config{})

	res := request(t, http.MethodPost, s.URL+mountPrefix+"/api/seed/users.yaml", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
	seeded := decode[httpapi.SeedResponse](t, res)
	assert.True(t, len(seeded.Tables) > 0)
	for _, table := range seeded.Tables {
		assert.True(t, table.Success, "%s %s: %s", table.Task, table.Table, table.Error)
	}

	// The rows landed in the pool the test owns, not in a second one.
	var count int
	assert.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM member").Scan(&count))
	assert.Equal(t, 2, count)

	res = request(t, http.MethodGet, s.URL+mountPrefix+"/api/assert/expect-ok.yaml", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	matched := decode[httpapi.AssertResponse](t, res)
	assert.Equal(t, 1, len(matched.Tables))
	assert.True(t, matched.Tables[0].Match)
}

// TestHandlerAssertMismatchIs400 pins the contract a Playwright harness
// branches on: response.ok() is false, and the body is still AssertResponse.
func TestHandlerAssertMismatchIs400(t *testing.T) {
	s, _ := newTestServer(t, httpapi.Config{})

	request(t, http.MethodPost, s.URL+mountPrefix+"/api/seed/users.yaml", nil)

	res := request(t, http.MethodGet, s.URL+mountPrefix+"/api/assert/expect-ng.yaml", nil)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
	result := decode[httpapi.AssertResponse](t, res)
	assert.Equal(t, 1, len(result.Tables))
	assert.False(t, result.Tables[0].Match)
	assert.True(t, len(result.Tables[0].Diff) > 0)
}

func TestHandlerSeedOptions(t *testing.T) {
	s, db := newTestServer(t, httpapi.Config{})

	body := strings.NewReader(`{"include_tags": ["admin"]}`)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.URL+mountPrefix+"/api/seed/users.yaml", body)
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	var count int
	assert.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM member").Scan(&count))
	assert.Equal(t, 1, count)

	// A dataset in a subdirectory is addressable by its slash separated path.
	res = request(t, http.MethodGet, s.URL+mountPrefix+"/api/assert/nested/expect-admin.yaml", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestHandlerList(t *testing.T) {
	s, _ := newTestServer(t, httpapi.Config{})

	res := request(t, http.MethodGet, s.URL+mountPrefix+"/api/list", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	list := decode[httpapi.ListResult](t, res)
	assert.Equal(t, []string{"expect-ng.yaml", "expect-ok.yaml", "nested/expect-admin.yaml", "users.yaml"}, list.DataSets)

	// The plain text form advertises the URLs the handler is actually mounted
	// at, prefix included.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.URL+mountPrefix+"/api/list", nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	text, err := io.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(text), s.URL+mountPrefix+"/api/seed/users.yaml")
}

// TestHandlerRejectsPathEscape covers the guarantee an embedder needs before
// serving this on an application listener. Redirects are not followed, because
// a redirect to a cleaned path is itself a refusal to serve what was asked for.
func TestHandlerRejectsPathEscape(t *testing.T) {
	counting := &countingFS{FS: dataSets}
	s, _ := newTestServer(t, httpapi.Config{DataSets: counting})

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for _, path := range []string{
		"../../etc/passwd",
		"..%2f..%2fetc%2fpasswd",
		"%2e%2e/%2e%2e/etc/passwd",
		"..%5c..%5cwindows",
		"nested/../../users.yaml",
		"/etc/passwd",
	} {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.URL+mountPrefix+"/api/assert/"+path, nil)
			assert.NoError(t, err)
			req.Header.Set("Accept", "application/json")
			res, err := client.Do(req)
			assert.NoError(t, err)
			defer res.Body.Close()
			assert.True(t, res.StatusCode >= 300, "status = %d", res.StatusCode)
			assert.Equal(t, 0, counting.opened, "the file system was touched")
		})
	}
}

// TestHandlerJSONErrors keeps a JSON client from being handed prose.
func TestHandlerJSONErrors(t *testing.T) {
	s, _ := newTestServer(t, httpapi.Config{AlwaysJSON: true})

	res := request(t, http.MethodGet, s.URL+mountPrefix+"/api/assert/..%2fescape.yaml", nil)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
	assert.True(t, decode[httpapi.ErrorResponse](t, res).Error != "")

	// AlwaysJSON answers with JSON even when the client asked for nothing.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.URL+mountPrefix+"/api/seed/missing.yaml", nil)
	assert.NoError(t, err)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
	assert.True(t, decode[httpapi.ErrorResponse](t, res).Error != "")
}

// TestHandlerTextErrors keeps the pre-v0.4.0 behavior for a plain text client.
func TestHandlerTextErrors(t *testing.T) {
	s, _ := newTestServer(t, httpapi.Config{})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.URL+mountPrefix+"/api/seed/missing.yaml", nil)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/plain")
	body, err := io.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "preparation error")
}

// TestHandlerConnectorPerRequest shows the transaction-scoped use: the handler
// asks for a connector once per request and uses whatever it gets.
func TestHandlerConnectorPerRequest(t *testing.T) {
	var calls int
	var tx *sql.Tx

	s, db := newTestServer(t, httpapi.Config{
		Connector: func(context.Context) (dbtestify.DBConnector, error) {
			calls++
			return dbtestify.NewDBConnectorFromTx(tx, dbtestify.SQLiteDialect)
		},
	})

	var err error
	tx, err = db.BeginTx(t.Context(), nil)
	assert.NoError(t, err)
	defer tx.Rollback()

	res := request(t, http.MethodPost, s.URL+mountPrefix+"/api/seed/users.yaml", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, 1, calls)

	// Uncommitted rows are visible to an assert on the same transaction.
	res = request(t, http.MethodGet, s.URL+mountPrefix+"/api/assert/expect-ok.yaml", nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, 2, calls)

	assert.NoError(t, tx.Rollback())
	var count int
	assert.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM member").Scan(&count))
	assert.Equal(t, 0, count)
}

func TestNewHandlerRejectsIncompleteConfig(t *testing.T) {
	_, err := httpapi.NewHandler(httpapi.Config{DataSets: dataSets})
	assert.IsError(t, err, httpapi.ErrInvalidConfig)

	_, err = httpapi.NewHandler(httpapi.Config{
		Connector: func(context.Context) (dbtestify.DBConnector, error) { return nil, nil },
	})
	assert.IsError(t, err, httpapi.ErrInvalidConfig)
}
