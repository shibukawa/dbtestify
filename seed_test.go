package dbtestify

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	_ "github.com/shibukawa/dbtestify/drivers"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestSeedSQLite(t *testing.T) {
	type args struct {
		src string
		opt SeedOpt
	}
	tests := []struct {
		name       string
		args       args
		wantNames  []string
		wantEmails []any
		wantErr    bool
	}{
		{
			name: "insert operation",
			args: args{
				src: TrimIndent(t, `
					user:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 4, name: Ivan } # no email
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"user": InsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Ivan", "John", "Kate"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil, "john@example.com", nil},
		},
		{
			name: "delete operation (success)",
			args: args{
				src: TrimIndent(t, `
					user:
					- { id: 5 } # John
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"user": DeleteOperation},
				},
			},
			wantNames:  []string{"Kate"},
			wantEmails: []any{nil},
		},
		{
			name: "delete operation (missing)",
			args: args{
				src: TrimIndent(t, `
					user:
					- { id: 7 } # missing
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"user": DeleteOperation},
				},
			},
			wantNames:  []string{"John", "Kate"},
			wantEmails: []any{"john@example.com", nil},
		},
		{
			name: "truncate operation",
			args: args{
				src: "",
				opt: SeedOpt{
					Operations: map[string]Operation{"user": TruncateOperation},
				},
			},
			wantNames:  nil,
			wantEmails: nil,
		},
		{
			name: "clear-insert operation",
			args: args{
				src: TrimIndent(t, `
					user:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 4, name: Ivan } # no email
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"user": ClearInsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Ivan"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil},
		},
		{
			name: "upsert operation",
			args: args{
				src: TrimIndent(t, `
					user:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 5, name: Johnny } # update
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"user": UpsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Johnny", "Kate"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil, nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Remove("seed_clear_insert.db")
			connStr := "file:seed_clear_insert.db?cache=shared&mode=rwc"

			ctx2, cancel := context.WithCancel(t.Context())
			defer cancel()

			dbc, err := NewDBConnector(ctx2, "sqlite3://"+connStr)
			assert.NoError(t, err)

			_, err = dbc.DB().ExecContext(t.Context(), TrimIndent(t, `
				CREATE TABLE IF NOT EXISTS user (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					email TEXT UNIQUE
				);

				DELETE FROM user;

				INSERT INTO user (id, name, email)
				VALUES
					(5, 'John', 'john@example.com'),
					(6, 'Kate', null);
			`))
			assert.NoError(t, err)

			data, err := ParseYAML(strings.NewReader(tt.args.src))
			assert.NoError(t, err)

			if err := Seed(t.Context(), dbc, data, tt.args.opt); (err != nil) != tt.wantErr {
				t.Errorf("Seed() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				rows, err := dbc.DB().QueryContext(t.Context(), TrimIndent(t, `
					SELECT
						u.name,
						u.email
					FROM
						user AS u
					ORDER BY
						u.name;
				`))
				assert.NoError(t, err)
				var names []string
				var emails []any
				for rows.Next() {
					var name string
					var email sql.NullString
					err := rows.Scan(&name, &email)
					assert.NoError(t, err)
					names = append(names, name)
					if email.Valid {
						emails = append(emails, email.String)
					} else {
						emails = append(emails, nil)
					}
				}
				assert.Equal(t, tt.wantNames, names)
				assert.Equal(t, tt.wantEmails, emails)
			}
		})
	}
}

func TestSeedPostgreSQL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	type args struct {
		src string
		opt SeedOpt
	}
	tests := []struct {
		name       string
		args       args
		wantNames  []string
		wantEmails []any
		wantErr    bool
	}{
		{
			name: "insert operation",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 4, name: Ivan } # no email
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": InsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Ivan", "John", "Kate"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil, "john@example.com", nil},
		},
		{
			name: "delete operation (success)",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 5 } # John
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": DeleteOperation},
				},
			},
			wantNames:  []string{"Kate"},
			wantEmails: []any{nil},
		},
		{
			name: "delete operation (missing)",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 7 } # missing
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": DeleteOperation},
				},
			},
			wantNames:  []string{"John", "Kate"},
			wantEmails: []any{"john@example.com", nil},
		},
		{
			name: "truncate operation",
			args: args{
				src: "",
				opt: SeedOpt{
					Operations: map[string]Operation{"member": TruncateOperation},
				},
			},
			wantNames:  nil,
			wantEmails: nil,
		},
		{
			name: "clear-insert operation",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 4, name: Ivan } # no email
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": ClearInsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Ivan"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil},
		},
		{
			name: "upsert operation",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 5, name: Johnny } # update
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": UpsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Johnny", "Kate"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil, nil},
		},
	}

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:15.3-alpine",
		postgres.WithInitScripts(filepath.Join("testdata", "seed-test-init.sql")),
		postgres.WithDatabase("seedtest"),
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx2, cancel := context.WithCancel(context.Background())
			defer cancel()

			dbc, err := NewDBConnector(ctx2, connStr)
			assert.NoError(t, err)

			dbc.DB().ExecContext(ctx2, TrimIndent(t, `
				TRUNCATE TABLE member;

				INSERT INTO
					member (id, name, email)
				VALUES
					(5, 'John', 'john@example.com'),
					(6, 'Kate', null);
			`))

			data, err := ParseYAML(strings.NewReader(tt.args.src))
			assert.NoError(t, err)

			if err := Seed(t.Context(), dbc, data, tt.args.opt); (err != nil) != tt.wantErr {
				t.Errorf("Seed() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				rows, err := dbc.DB().QueryContext(t.Context(), TrimIndent(t, `
					SELECT
						m.name,
						m.email
					FROM
						member AS m
					ORDER BY
						m.name;
				`))
				assert.NoError(t, err)
				var names []string
				var emails []any
				for rows.Next() {
					var name string
					var email sql.NullString
					err := rows.Scan(&name, &email)
					assert.NoError(t, err)
					names = append(names, name)
					if email.Valid {
						emails = append(emails, email.String)
					} else {
						emails = append(emails, nil)
					}
				}
				assert.Equal(t, tt.wantNames, names)
				assert.Equal(t, tt.wantEmails, emails)
			}
		})
	}
}

func TestSeedMySQL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	type args struct {
		src string
		opt SeedOpt
	}
	tests := []struct {
		name       string
		args       args
		wantNames  []string
		wantEmails []any
		wantErr    bool
	}{
		{
			name: "insert operation",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 4, name: Ivan } # no email
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": InsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Ivan", "John", "Kate"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil, "john@example.com", nil},
		},
		{
			name: "delete operation (success)",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 5 } # John
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": DeleteOperation},
				},
			},
			wantNames:  []string{"Kate"},
			wantEmails: []any{nil},
		},
		{
			name: "delete operation (missing)",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 7 } # missing
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": DeleteOperation},
				},
			},
			wantNames:  []string{"John", "Kate"},
			wantEmails: []any{"john@example.com", nil},
		},
		{
			name: "truncate operation",
			args: args{
				src: "",
				opt: SeedOpt{
					Operations: map[string]Operation{"member": TruncateOperation},
				},
			},
			wantNames:  nil,
			wantEmails: nil,
		},
		{
			name: "clear-insert operation",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 4, name: Ivan } # no email
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": ClearInsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Ivan"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil},
		},
		{
			name: "upsert operation",
			args: args{
				src: TrimIndent(t, `
					member:
					- { id: 1, name: Frank, email: frank@example.com }
					- { id: 2, name: Grace, email: grace@example.com }
					- { id: 3, name: Heidi, email: heidi@example.com }
					- { id: 5, name: Johnny } # update
					`),
				opt: SeedOpt{
					Operations: map[string]Operation{"member": UpsertOperation},
				},
			},
			wantNames:  []string{"Frank", "Grace", "Heidi", "Johnny", "Kate"},
			wantEmails: []any{"frank@example.com", "grace@example.com", "heidi@example.com", nil, nil},
		},
	}

	ctx := context.Background()

	mysqlContainer, err := mysql.Run(ctx, "mysql:8",
		mysql.WithDatabase("seedtest"),
		mysql.WithUsername("root"),
		mysql.WithPassword("password"),
		mysql.WithScripts(filepath.Join("testdata", "seed-test-init.sql")),
	)
	assert.NoError(t, err)
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(mysqlContainer); err != nil {
			t.Fatalf("failed to terminate mysqlContainer: %s", err)
		}
	})
	connStr, err := mysqlContainer.ConnectionString(ctx, "tls=skip-verify")
	assert.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx2, cancel := context.WithCancel(context.Background())
			defer cancel()

			dbc, err := NewDBConnector(ctx2, "mysql://"+connStr)
			assert.NoError(t, err)

			dbc.DB().ExecContext(ctx2, TrimIndent(t, `
				TRUNCATE TABLE member;
			`))

			dbc.DB().ExecContext(ctx2, TrimIndent(t, `
				INSERT INTO
					member (id, name, email)
				VALUES
					(5, 'John', 'john@example.com'),
					(6, 'Kate', null);
			`))

			data, err := ParseYAML(strings.NewReader(tt.args.src))
			assert.NoError(t, err)
			if err := Seed(t.Context(), dbc, data, tt.args.opt); (err != nil) != tt.wantErr {
				t.Errorf("Seed() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				rows, err := dbc.DB().QueryContext(t.Context(), TrimIndent(t, `
					SELECT
						m.name,
						m.email
					FROM
						member AS m
					ORDER BY
						m.name;
				`))
				assert.NoError(t, err)
				var names []string
				var emails []any
				for rows.Next() {
					var name string
					var email sql.NullString
					err := rows.Scan(&name, &email)
					assert.NoError(t, err)
					names = append(names, name)
					if email.Valid {
						emails = append(emails, email.String)
					} else {
						emails = append(emails, nil)
					}
				}
				assert.Equal(t, tt.wantNames, names)
				assert.Equal(t, tt.wantEmails, emails)
			}
		})
	}
}
