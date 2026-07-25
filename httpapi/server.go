package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shibukawa/dbtestify"
	_ "github.com/shibukawa/dbtestify/drivers"
)

const MAX_MEMORY_BYTES = 1024 * 1024

func jsonAcceptable(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func Start(ctx context.Context, dir, dbconn string, port uint16) error {
	// check parameter
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	if len(getTestList(root.FS())) == 0 {
		return fmt.Errorf("No data set found in '%s'. Data set should be YAML file.", dir)
	}
	err = testDBConnection(ctx, dbconn)
	if err != nil {
		return err
	}

	m := http.NewServeMux()
	m.HandleFunc("GET /api/list", func(w http.ResponseWriter, r *http.Request) {
		useJson := jsonAcceptable(r)
		if useJson {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
		dumpDataSetList(useJson, w, root.FS(), port)
	})

	m.HandleFunc("POST /api/seed/{path...}", func(w http.ResponseWriter, r *http.Request) {
		useJson := jsonAcceptable(r)

		opt, err := parseSeedRequest(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error parsing request: %v", err), http.StatusBadRequest)
			return
		}
		path := r.PathValue("path")
		if useJson {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
		dbc, err := dbtestify.NewDBConnector(ctx, dbconn)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain")
			http.Error(w, fmt.Sprintf(`database connection error: %v`, err), http.StatusInternalServerError)
			return
		}
		err = seedTable(r.Context(), dbc, useJson, w, filepath.Join(dir, path), *opt)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain")
			http.Error(w, fmt.Sprintf("preparation error: %v", err), http.StatusInternalServerError)
		}
	})

	m.HandleFunc("GET /api/assert/{path...}", func(w http.ResponseWriter, r *http.Request) {
		useJson := jsonAcceptable(r)

		opt := parseAssertRequest(r)

		path := r.PathValue("path")
		if useJson {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
		dbc, err := dbtestify.NewDBConnector(ctx, dbconn)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain")
			http.Error(w, fmt.Sprintf(`database connection error: %v`, err), http.StatusInternalServerError)
			return
		}
		_, err = assertTable(r.Context(), dbc, useJson, w, filepath.Join(dir, path), opt)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain")
			http.Error(w, fmt.Sprintf("assert error: %v", err), http.StatusInternalServerError)
		}
	})

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(int(port)),
		Handler: m,
	}
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Shutdown(ctx)
	}()
	fmt.Printf(`dbtestify API server
	
	GET  http://localhost:%[1]d/api/list                    : Show data set file list
	POST http://localhost:%[1]d/api/seed/{data set path}    : Seed database content with the specified data set
	GET  http://localhost:%[1]d/api/assert/{data set path}  : Assert database content with the specified data set
	`, port)

	fmt.Printf("start receiving at :%d\n", port)
	return s.ListenAndServe()
}

func parseSeedRequest(r *http.Request) (*SeedOpt, error) {
	contentType := r.Header.Get("Content-Type")
	var opt SeedOpt
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		decoder := json.NewDecoder(r.Body)
		defer r.Body.Close()
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&opt)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		fallthrough
	case strings.HasPrefix(contentType, "multipart/form-data"):
		if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
			err := r.ParseForm()
			if err != nil {
				return nil, err
			}
		} else {
			err := r.ParseMultipartForm(MAX_MEMORY_BYTES)
			if err != nil {
				return nil, err
			}
		}
		opt.IncludeTags = append(r.Form["i"], r.Form["include_tag"]...)
		opt.ExcludeTags = append(r.Form["e"], r.Form["exclude_tag"]...)
		opt.Targets = append(r.Form["t"], r.Form["target"]...)
		opt.Truncates = r.Form["truncate"]
		if batchSize := r.Form.Get("batch_size"); batchSize != "" {
			batchSizeInt, err := strconv.Atoi(batchSize)
			if err != nil {
				return nil, err
			}
			opt.BatchSize = batchSizeInt
		}
	}
	slices.Sort(opt.IncludeTags)
	slices.Sort(opt.ExcludeTags)
	slices.Sort(opt.Targets)
	slices.Sort(opt.Truncates)
	if opt.BatchSize == 0 {
		opt.BatchSize = 50
	}
	return &opt, nil
}

func parseAssertRequest(r *http.Request) AssertOpt {
	params := r.URL.Query()
	opt := AssertOpt{
		IncludeTags: append(params["i"], params["include-tag"]...),
		ExcludeTags: append(params["e"], params["exclude-tag"]...),
		Targets:     append(params["t"], params["target"]...),
	}
	slices.Sort(opt.IncludeTags)
	slices.Sort(opt.ExcludeTags)
	slices.Sort(opt.Targets)
	return opt
}

func testDBConnection(ctx context.Context, dbconn string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dbc, err := dbtestify.NewDBConnector(ctx, dbconn)
	if err != nil {
		return err
	}
	_, err = dbc.TableNames(ctx)
	return err
}
