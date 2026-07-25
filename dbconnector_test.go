package dbtestify

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/docker/docker/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestDBConnectorPostgreSQL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	if !dockerAvailable() {
		t.Skip("Docker is not available; skipping container-based tests")
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:15.3-alpine",
		postgres.WithInitScripts(filepath.Join("testdata", "db-connector-test-init.sql")),
		postgres.WithDatabase("dbconnectortest"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate pgContainer: %s", err)
		}
	})
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	ctx2, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := openTestDBConnector(ctx2, connStr)
	assert.NoError(t, err)

	// with schema
	tnames, err := db.TableNames(ctx2, "public")
	assert.NoError(t, err)
	assert.Equal(t, []string{"book_authors", "orders", "student_course_enrollments"}, tnames)

	// without schema
	tnames, err = db.TableNames(ctx2)
	assert.NoError(t, err)
	assert.Equal(t, []string{"book_authors", "orders", "student_course_enrollments"}, tnames)

	// with schema
	pkeys, err := db.PrimaryKeys(ctx2, "public.orders")
	assert.NoError(t, err)
	assert.Equal(t, []string{"order_id", "product_id"}, pkeys)

	// without schema
	pkeys, err = db.PrimaryKeys(ctx2, "orders")
	assert.NoError(t, err)
	assert.Equal(t, []string{"order_id", "product_id"}, pkeys)
}

func TestDBConnectionMySQL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	if !dockerAvailable() {
		t.Skip("Docker is not available; skipping container-based tests")
	}

	ctx := context.Background()

	mysqlContainer, err := mysql.Run(ctx, "mysql:8",
		mysql.WithDatabase("foo"),
		mysql.WithUsername("root"),
		mysql.WithPassword("password"),
		mysql.WithScripts(filepath.Join("testdata", "db-connector-test-init.sql")),
	)
	assert.NoError(t, err)
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(mysqlContainer); err != nil {
			t.Fatalf("failed to terminate mysqlContainer: %s", err)
		}
	})
	connStr, err := mysqlContainer.ConnectionString(ctx, "tls=skip-verify")
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := openTestDBConnector(ctx, "mysql://"+connStr)
	assert.NoError(t, err)

	// with schema
	tnames, err := db.TableNames(ctx, "foo")
	assert.NoError(t, err)
	assert.Equal(t, []string{"book_authors", "orders", "student_course_enrollments"}, tnames)

	// without schema
	tnames, err = db.TableNames(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"book_authors", "orders", "student_course_enrollments"}, tnames)

	// with schema
	pkeys, err := db.PrimaryKeys(ctx, "foo.orders")
	assert.NoError(t, err)
	assert.Equal(t, []string{"order_id", "product_id"}, pkeys)

	// without schema
	pkeys, err = db.PrimaryKeys(ctx, "orders")
	assert.NoError(t, err)
	assert.Equal(t, []string{"order_id", "product_id"}, pkeys)
}

// dockerAvailable returns true if local Docker daemon is reachable within ~1s.
func dockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	_, err = cli.Ping(ctx)
	return err == nil
}

func TestDBConnectionSQLite(t *testing.T) {
	os.Remove("get_db_status.db")
	connStr := "file:get_db_status.db?cache=shared&mode=rwc"
	err := func() error {
		db, err := sql.Open("sqlite3", connStr)
		if err != nil {
			return err
		}
		defer db.Close()
		initSql, err := os.ReadFile(filepath.Join("testdata", "db-connector-test-init.sql"))
		if err != nil {
			return err
		}
		_, err = db.Exec(string(initSql))
		return err
	}()

	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := openTestDBConnector(ctx, "sqlite://"+connStr)
	assert.NoError(t, err)

	tnames, err := db.TableNames(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"book_authors", "orders", "student_course_enrollments"}, tnames)

	pkeys, err := db.PrimaryKeys(ctx, "orders")
	assert.NoError(t, err)
	assert.Equal(t, []string{"order_id", "product_id"}, pkeys)
}
