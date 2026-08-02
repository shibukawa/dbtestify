// Package serve runs the dbtestify HTTP API as a standalone server.
//
// It is separate from github.com/shibukawa/dbtestify/httpapi because it opens
// the database from a connection string and therefore registers the SQLite,
// PostgreSQL, and MySQL drivers. Embedders that already own a pool use
// httpapi.NewHandler instead and never pay for those dependencies.
package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/shibukawa/dbtestify"
	"github.com/shibukawa/dbtestify/connect"
	"github.com/shibukawa/dbtestify/httpapi"
)

// Start serves the dbtestify API on port until ctx is done.
//
// dir is the dataset root and dbconn is a connection string accepted by
// github.com/shibukawa/dbtestify/connect. Both are checked before the listener
// opens, so a bad directory or an unreachable database is an error from Start
// rather than a failure on the first request.
func Start(ctx context.Context, dir, dbconn string, port uint16) error {
	cfg, err := newConfig(ctx, dir, dbconn)
	if err != nil {
		return err
	}
	h, err := httpapi.NewHandler(cfg)
	if err != nil {
		return err
	}

	s := &http.Server{
		Addr:    ":" + strconv.Itoa(int(port)),
		Handler: h,
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

// newConfig validates the parameters and opens the one connection the server
// uses for its whole lifetime. Connecting per request would leave a pool and a
// goroutine behind for every seed and assert call.
func newConfig(ctx context.Context, dir, dbconn string) (httpapi.Config, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return httpapi.Config{}, err
	}
	dataSets := root.FS()
	if len(httpapi.ListDataSets(dataSets)) == 0 {
		return httpapi.Config{}, fmt.Errorf("No data set found in '%s'. Data set should be YAML file.", dir)
	}
	dbc, err := connect.NewDBConnector(ctx, dbconn)
	if err != nil {
		return httpapi.Config{}, err
	}
	if _, err := dbc.TableNames(ctx); err != nil {
		return httpapi.Config{}, err
	}
	return httpapi.Config{
		Connector: func(context.Context) (dbtestify.DBConnector, error) {
			return dbc, nil
		},
		DataSets: dataSets,
	}, nil
}
