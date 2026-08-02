package httpapi_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoDatabaseDriverDependency is the constraint that made this package
// embeddable: an application that builds with CGO_ENABLED=0, or that gets its
// SQLite driver elsewhere, must not link ours by importing the API.
func TestNoDatabaseDriverDependency(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "github.com/shibukawa/dbtestify/httpapi").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	for _, driver := range []string{
		"github.com/mattn/go-sqlite3",
		"github.com/go-sql-driver/mysql",
		"github.com/jackc/pgx/v5/stdlib",
		"github.com/shibukawa/dbtestify/connect",
	} {
		for _, dep := range strings.Fields(string(out)) {
			if dep == driver {
				t.Errorf("httpapi depends on %s", driver)
			}
		}
	}
}
