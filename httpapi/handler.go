package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/shibukawa/dbtestify"
)

// ErrInvalidConfig is returned by [NewHandler] when [Config] is incomplete.
var ErrInvalidConfig = errors.New("invalid config")

// ErrInvalidDataSetPath is returned to the client when a request names a
// dataset outside [Config.DataSets].
var ErrInvalidDataSetPath = errors.New("invalid data set path")

// Config configures a handler that serves the dbtestify HTTP API over
// resources the caller owns.
type Config struct {
	// Connector returns the DBConnector to use for one request.
	//
	// Returning the same connector every time is fine, and is what the
	// standalone server does: the handler never closes it, so the number of
	// pools does not grow with the number of requests. Returning a
	// transaction-backed connector per request is what lets an embedder scope
	// seeding to a transaction it controls. Required.
	Connector func(ctx context.Context) (dbtestify.DBConnector, error)

	// DataSets is the dataset root. Request paths are resolved inside it,
	// so no request can name a file outside. Required.
	DataSets fs.FS

	// AlwaysJSON responds with JSON regardless of the Accept header.
	AlwaysJSON bool
}

// ErrorResponse is the body of a failed request when the response is JSON.
type ErrorResponse struct {
	Error string `json:"error"`
}

type handler struct {
	connector  func(ctx context.Context) (dbtestify.DBConnector, error)
	dataSets   fs.FS
	alwaysJSON bool
}

// NewHandler returns the seed, assert, and list handlers registered on their
// current paths, relative to wherever the caller mounts them.
//
// The returned handler's patterns are relative, so [net/http.StripPrefix] is
// enough to serve them under a prefix:
//
//	h, err := httpapi.NewHandler(httpapi.Config{
//		Connector: func(ctx context.Context) (dbtestify.DBConnector, error) {
//			return dbtestify.NewDBConnectorFromDB(pool, dbtestify.PostgresDialect)
//		},
//		DataSets:   os.DirFS("testdata"),
//		AlwaysJSON: true,
//	})
//	mux.Handle("/_e2e/api/", http.StripPrefix("/_e2e", h))
func NewHandler(cfg Config) (http.Handler, error) {
	if cfg.Connector == nil {
		return nil, fmt.Errorf("%w: Connector is required", ErrInvalidConfig)
	}
	if cfg.DataSets == nil {
		return nil, fmt.Errorf("%w: DataSets is required", ErrInvalidConfig)
	}
	h := &handler{
		connector:  cfg.Connector,
		dataSets:   cfg.DataSets,
		alwaysJSON: cfg.AlwaysJSON,
	}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/list", h.list)
	m.HandleFunc("POST /api/seed/{path...}", h.seed)
	m.HandleFunc("GET /api/assert/{path...}", h.assert)
	return m, nil
}

func (h *handler) useJSON(r *http.Request) bool {
	return h.alwaysJSON || jsonAcceptable(r)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	useJson := h.useJSON(r)
	setContentType(w, useJson)
	dumpDataSetList(useJson, w, h.dataSets, baseURL(r))
}

func (h *handler) seed(w http.ResponseWriter, r *http.Request) {
	useJson := h.useJSON(r)

	opt, err := parseSeedRequest(r)
	if err != nil {
		writeError(w, useJson, http.StatusBadRequest, "Error parsing request: %v", err)
		return
	}
	path, err := dataSetPath(r.PathValue("path"))
	if err != nil {
		writeError(w, useJson, http.StatusBadRequest, "%v", err)
		return
	}
	dbc, err := h.connector(r.Context())
	if err != nil {
		writeError(w, useJson, http.StatusInternalServerError, "database connection error: %v", err)
		return
	}
	setContentType(w, useJson)
	err = seedTable(r.Context(), dbc, useJson, w, h.dataSets, path, *opt)
	if err != nil {
		writeError(w, useJson, http.StatusInternalServerError, "preparation error: %v", err)
	}
}

func (h *handler) assert(w http.ResponseWriter, r *http.Request) {
	useJson := h.useJSON(r)

	opt := parseAssertRequest(r)

	path, err := dataSetPath(r.PathValue("path"))
	if err != nil {
		writeError(w, useJson, http.StatusBadRequest, "%v", err)
		return
	}
	dbc, err := h.connector(r.Context())
	if err != nil {
		writeError(w, useJson, http.StatusInternalServerError, "database connection error: %v", err)
		return
	}
	setContentType(w, useJson)
	_, err = assertTable(r.Context(), dbc, useJson, w, h.dataSets, path, opt)
	if err != nil {
		writeError(w, useJson, http.StatusInternalServerError, "assert error: %v", err)
	}
}

// dataSetPath rejects a request path that could name a file outside the
// dataset root before anything opens it. fs.FS paths are always slash
// separated, relative, and free of "." and ".." elements, so a path that
// survives this check cannot escape [Config.DataSets].
func dataSetPath(path string) (string, error) {
	if path == "" || !fs.ValidPath(path) || strings.ContainsRune(path, '\\') {
		return "", fmt.Errorf("%w: '%s'", ErrInvalidDataSetPath, path)
	}
	return path, nil
}

func setContentType(w http.ResponseWriter, useJson bool) {
	if useJson {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain")
	}
}

func writeError(w http.ResponseWriter, useJson bool, code int, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if useJson {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(&ErrorResponse{Error: message})
		return
	}
	http.Error(w, message, code)
}

// baseURL rebuilds the URL this handler is mounted at, so the plain text
// dataset list can print copy-pastable curl commands. r.RequestURI still holds
// the path the client asked for even when http.StripPrefix has rewritten
// r.URL.Path, and the difference between them is the mount prefix.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	var prefix string
	if u, err := url.ParseRequestURI(r.RequestURI); err == nil {
		prefix = strings.TrimSuffix(u.Path, r.URL.Path)
	}
	return scheme + "://" + host + prefix
}
