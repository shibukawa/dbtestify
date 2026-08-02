// Package httpapi serves the dbtestify seed and assert API over HTTP.
//
// [NewHandler] returns an [net/http.Handler] built on resources the caller
// owns: a [github.com/shibukawa/dbtestify.DBConnector] and an [io/fs.FS] that
// holds the datasets. An application under test can therefore mount the API on
// its own listener, behind a build tag, and seed the very pool it uses itself.
// That is the only way to reach an in-memory SQLite database, and it avoids a
// second writer racing the application's on a file-backed one.
//
// This package registers no database/sql driver, so importing it does not link
// pgx, go-sql-driver/mysql, or the cgo-dependent mattn/go-sqlite3. To run the
// same API as a standalone server that opens a connection from a connection
// string, use github.com/shibukawa/dbtestify/httpapi/serve.
package httpapi
