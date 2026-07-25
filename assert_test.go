package dbtestify

import (
	"context"
	"database/sql"
	"embed"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/goccy/go-yaml"
)

type Opt struct {
	SeedIncludeTags   []string `yaml:"seedIncludeTags"`
	SeedExcludeTags   []string `yaml:"seedExcludeTags"`
	SeedTargets       []string `yaml:"seedTargets"`
	AssertIncludeTags []string `yaml:"assertIncludeTags"`
	AssertExcludeTags []string `yaml:"assertExcludeTags"`
	AssertTargets     []string `yaml:"assertTargets"`
}

//go:embed testdata/assert-testcases
var testcaseDir embed.FS

func TestAssertSQLite(t *testing.T) {
	testcases, err := testcaseDir.ReadDir("testdata/assert-testcases")
	if err != nil {
		panic(err)
	}
	for _, testcase := range testcases {
		t.Run(testcase.Name(), func(t *testing.T) {
			os.Remove("assert_test.db")
			connStr := "file:assert_test.db?cache=shared&mode=rwc"

			ctx2, cancel := context.WithCancel(t.Context())
			defer cancel()

			dbc, err := openTestDBConnector(ctx2, "sqlite3://"+connStr)
			assert.NoError(t, err)

			_, err = dbc.DB().ExecContext(t.Context(), TrimIndent(t, `
				CREATE TABLE IF NOT EXISTS member (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL,
					email TEXT UNIQUE
				);

				CREATE TABLE IF NOT EXISTS dummy (
					id INTEGER PRIMARY KEY AUTOINCREMENT
				);
				`))
			assert.NoError(t, err)

			var opt Opt
			optFile, err := testcaseDir.Open(filepath.Join("testdata/assert-testcases", testcase.Name(), "opt.yaml"))
			if err == nil {
				err = yaml.NewDecoder(optFile).Decode(&opt)
				assert.NoError(t, err)
			}

			srcFile, err := testcaseDir.Open(filepath.Join("testdata/assert-testcases", testcase.Name(), "seed.yaml"))
			assert.NoError(t, err)
			defer srcFile.Close()

			src, err := ParseYAML(srcFile)
			assert.NoError(t, err)

			err = Seed(t.Context(), dbc, src, SeedOpt{
				IncludeTags:  opt.SeedIncludeTags,
				ExcludeTags:  opt.SeedExcludeTags,
				TargetTables: opt.SeedTargets,
			})
			assert.NoError(t, err)

			expectFile, err := testcaseDir.Open(filepath.Join("testdata/assert-testcases", testcase.Name(), "assert.yaml"))
			assert.NoError(t, err)
			defer expectFile.Close()

			expect, err := ParseYAML(expectFile)
			assert.NoError(t, err)

			ok, result, err := Assert(t.Context(), dbc, expect, AssertOpt{
				IncludeTags:  opt.AssertIncludeTags,
				ExcludeTags:  opt.AssertExcludeTags,
				TargetTables: opt.AssertTargets,
			})
			assert.NoError(t, err)

			wantMatch := !strings.HasSuffix(testcase.Name(), "-ng")
			if ok != wantMatch {
				t.Error("Assert() wrong result")
				dump := DumpDiffCLICallback(true, false)
				for _, r := range result {
					dump(r)
				}
			}
		})
	}
}

func Test_fetchTableData(t *testing.T) {
	os.Remove("fetch_data_test.db")
	connStr := "file:fetch_data_test.db?cache=shared&mode=rwc"
	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS user (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL
		);

		INSERT INTO user (id, name, email)
		VALUES
			(2, 'Grace', 'grace@example.com'),
			(1, 'Frank', 'frank@example.com');
	`)
	assert.NoError(t, err)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbc, err := openTestDBConnector(ctx, "sqlite3://"+connStr)
	assert.NoError(t, err)

	actual, _, err := fetchTableData(ctx, dbc, "user")
	assert.NoError(t, err)

	expected := [][]Value{
		{Value{"id", 1}, Value{"email", "frank@example.com"}, Value{"name", "Frank"}},
		{Value{"id", 2}, Value{"email", "grace@example.com"}, Value{"name", "Grace"}},
	}
	assert.Equal(t, expected, actual)
}

func Test_compareRow(t *testing.T) {
	type args struct {
		offset   int
		expected []Value
		actual   []Value
	}
	tests := []struct {
		name          string
		args          args
		wantRowStatus AssertStatus
		wantDetail    RowDiff
	}{
		{
			name: "completely match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: 2, Actual: 2, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "not match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 3}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: 2, Actual: 3, Status: NotMatch},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only in actual (1): inside list: this is ignored",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key3", Value: 3}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}, {Key: "key3", Value: 3}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key3", Expect: 3, Actual: 3, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "only in actual (2): end of line: this is ignored",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}, {Key: "key3", Value: 3}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: 2, Actual: 2, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "only in expected (1): inside list: wrong-data-set",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}, {Key: "key3", Value: 3}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key3", Value: 3}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: 2, Status: WrongDataSet},
					{Key: "key3", Expect: 3, Actual: 3, Status: Match},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only in expected (2): end of line: wrong-data-set",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}, {Key: "key3", Value: 3}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: 2, Actual: 2, Status: Match},
					{Key: "key3", Expect: 3, Status: WrongDataSet},
				},
				Status: NotMatch,
			},
		},
		{
			name: "completely match: nil & nil",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: nil}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: nil}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: nil, Actual: nil, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "completely match: [null] placeholder (1): ok",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{"null"}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: nil}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{"null"}, Actual: nil, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "completely match: [null] placeholder (2): ng",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{"null"}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{"null"}, Actual: 2, Status: NotMatch},
				},
				Status: NotMatch,
			},
		},
		{
			name: "completely match: [null] placeholder (3): ok(primitive)",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{nil}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: nil}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{nil}, Actual: nil, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "completely match: [null] placeholder (4): ng(primitive)",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{nil}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 2}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{nil}, Actual: 2, Status: NotMatch},
				},
				Status: NotMatch,
			},
		},
		{
			name: "completely match: [notnull] placeholder (1): ok",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{"notnull"}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 3}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{"notnull"}, Actual: 3, Status: Match},
				},
				Status: Match,
			},
		},
		{
			name: "completely match: [notnull] placeholder (2): ng",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{"notnull"}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: nil}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{"notnull"}, Actual: nil, Status: NotMatch},
				},
				Status: NotMatch,
			},
		},
		{
			name: "completely match: [any] placeholder: ok",
			args: args{
				offset:   0,
				expected: []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: []any{"any"}}},
				actual:   []Value{{Key: "key1", Value: "value1"}, {Key: "key2", Value: 3}},
			},
			wantDetail: RowDiff{
				Fields: []Diff{
					{Key: "key1", Expect: "value1", Actual: "value1", Status: Match},
					{Key: "key2", Expect: []any{"any"}, Actual: 3, Status: Match},
				},
				Status: Match,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDetail := compareRow(tt.args.offset, tt.args.expected, tt.args.actual)
			if !reflect.DeepEqual(gotDetail, tt.wantDetail) {
				t.Errorf("compareRow() gotDetail = %v, want %v", gotDetail, tt.wantDetail)
			}
		})
	}
}

func Test_compareTable(t *testing.T) {
	type args struct {
		tableName string
		pkeys     []string
		strategy  MatchStrategy
		expected  [][]Value
		actual    [][]Value
	}
	tests := []struct {
		name string
		args args
		want AssertTableResult
	}{
		{
			name: "exact match with exact strategy: ok",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2", Actual: "key2", Status: Match},
							{Key: "value", Expect: 2, Actual: 2, Status: Match},
						},
						Status: Match,
					},
				},
				Status: Match,
			},
		},
		{
			name: "exact match with exact strategy: ng",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2", Actual: "key2", Status: Match},
							{Key: "value", Expect: 2, Actual: 3, Status: NotMatch},
						},
						Status: NotMatch,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "exact match with exact strategy: complex primary-key: ok",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key1", "key2"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key1", Value: "key1-1"}, {Key: "key2", Value: 1}, {Key: "value", Value: 1}},
					{{Key: "key1", Value: "key1-2"}, {Key: "key2", Value: 2}, {Key: "value", Value: 2}},
				},
				actual: [][]Value{
					{{Key: "key1", Value: "key1-1"}, {Key: "key2", Value: 1}, {Key: "value", Value: 1}},
					{{Key: "key1", Value: "key1-2"}, {Key: "key2", Value: 2}, {Key: "value", Value: 2}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key1", "key2"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key1", Expect: "key1-1", Actual: "key1-1", Status: Match},
							{Key: "key2", Expect: 1, Actual: 1, Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key1", Expect: "key1-2", Actual: "key1-2", Status: Match},
							{Key: "key2", Expect: 2, Actual: 2, Status: Match},
							{Key: "value", Expect: 2, Actual: 2, Status: Match},
						},
						Status: Match,
					},
				},
				Status: Match,
			},
		},
		{
			name: "exact match with exact strategy: complex primary-key: ng",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key1", "key2"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key1", Value: "key1-1"}, {Key: "key2", Value: 1}, {Key: "value", Value: 1}},
					{{Key: "key1", Value: "key1-2"}, {Key: "key2", Value: 2}, {Key: "value", Value: 2}},
				},
				actual: [][]Value{
					{{Key: "key1", Value: "key1-1"}, {Key: "key2", Value: 1}, {Key: "value", Value: 1}},
					{{Key: "key1", Value: "key1-2"}, {Key: "key2", Value: 2}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key1", "key2"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key1", Expect: "key1-1", Actual: "key1-1", Status: Match},
							{Key: "key2", Expect: 1, Actual: 1, Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key1", Expect: "key1-2", Actual: "key1-2", Status: Match},
							{Key: "key2", Expect: 2, Actual: 2, Status: Match},
							{Key: "value", Expect: 2, Actual: 3, Status: NotMatch},
						},
						Status: NotMatch,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on expect with exact strategy (1): inside row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2"},
							{Key: "value", Expect: 2},
						},
						Status: OnlyOnExpect,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key3", Actual: "key3", Status: Match},
							{Key: "value", Expect: 3, Actual: 3, Status: Match},
						},
						Status: Match,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on expect with exact strategy (2): end of row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2", Actual: "key2", Status: Match},
							{Key: "value", Expect: 2, Actual: 2, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key3"},
							{Key: "value", Expect: 3},
						},
						Status: OnlyOnExpect,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on expect with sub match strategy (1): inside row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  SubMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2"},
							{Key: "value", Expect: 2},
						},
						Status: OnlyOnExpect,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key3", Actual: "key3", Status: Match},
							{Key: "value", Expect: 3, Actual: 3, Status: Match},
						},
						Status: Match,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on expect with sub match strategy (2): end of row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  SubMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2", Actual: "key2", Status: Match},
							{Key: "value", Expect: 2, Actual: 2, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key3"},
							{Key: "value", Expect: 3},
						},
						Status: OnlyOnExpect,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on actual with exact strategy (1): inside row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Actual: "key2"},
							{Key: "value", Actual: 2},
						},
						Status: OnlyOnActual,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key3", Actual: "key3", Status: Match},
							{Key: "value", Expect: 3, Actual: 3, Status: Match},
						},
						Status: Match,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on actual with exact strategy (2): end of row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  ExactMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2", Actual: "key2", Status: Match},
							{Key: "value", Expect: 2, Actual: 2, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Actual: "key3"},
							{Key: "value", Actual: 3},
						},
						Status: OnlyOnActual,
					},
				},
				Status: NotMatch,
			},
		},
		{
			name: "only on actual with sub match strategy (1): inside row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  SubMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key3", Actual: "key3", Status: Match},
							{Key: "value", Expect: 3, Actual: 3, Status: Match},
						},
						Status: Match,
					},
				},
				Status: Match,
			},
		},
		{
			name: "only on actual with sub match strategy (2): end of row",
			args: args{
				tableName: "table1",
				pkeys:     []string{"key"},
				strategy:  SubMatchStrategy,
				expected: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
				},
				actual: [][]Value{
					{{Key: "key", Value: "key1"}, {Key: "value", Value: 1}},
					{{Key: "key", Value: "key2"}, {Key: "value", Value: 2}},
					{{Key: "key", Value: "key3"}, {Key: "value", Value: 3}},
				},
			},
			want: AssertTableResult{
				Name:        "table1",
				PrimaryKeys: []string{"key"},
				Rows: []RowDiff{
					{
						Fields: []Diff{
							{Key: "key", Expect: "key1", Actual: "key1", Status: Match},
							{Key: "value", Expect: 1, Actual: 1, Status: Match},
						},
						Status: Match,
					},
					{
						Fields: []Diff{
							{Key: "key", Expect: "key2", Actual: "key2", Status: Match},
							{Key: "value", Expect: 2, Actual: 2, Status: Match},
						},
						Status: Match,
					},
				},
				Status: Match,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareTable(tt.args.tableName, tt.args.strategy, tt.args.pkeys, tt.args.expected, tt.args.actual); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("compareTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compareRow_Directives_CurrentDate(t *testing.T) {
	// Freeze time
	oldTimeNow := timeNow
	timeNow = func() time.Time { return time.Date(2025, 9, 23, 12, 0, 0, 0, time.UTC) }
	defer func() { timeNow = oldTimeNow }()

	base := timeNow()

	type args struct {
		offset   int
		expected []Value
		actual   []Value
	}

	tests := []struct {
		name       string
		args       args
		wantStatus AssertStatus
	}{
		{
			name: "currentdate default 1m: within 30s => match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "ts", Value: []any{"currentdate"}}},
				actual:   []Value{{Key: "ts", Value: base.Add(30 * time.Second)}},
			},
			wantStatus: Match,
		},
		{
			name: "currentdate default 1m: over 90s => not match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "ts", Value: []any{"currentdate"}}},
				actual:   []Value{{Key: "ts", Value: base.Add(90 * time.Second)}},
			},
			wantStatus: NotMatch,
		},
		{
			name: "currentdate 2m: within 90s => match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "ts", Value: []any{"currentdate", "2m"}}},
				actual:   []Value{{Key: "ts", Value: base.Add(90 * time.Second)}},
			},
			wantStatus: Match,
		},
		{
			name: "currentdate parse string RFC3339: match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "ts", Value: []any{"currentdate"}}},
				actual:   []Value{{Key: "ts", Value: base.Add(20 * time.Second).Format(time.RFC3339)}},
			},
			wantStatus: Match,
		},
		{
			name: "currentdate cannot parse string: not match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "ts", Value: []any{"currentdate"}}},
				actual:   []Value{{Key: "ts", Value: "not-a-time"}},
			},
			wantStatus: NotMatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareRow(tt.args.offset, tt.args.expected, tt.args.actual)
			if got.Status != tt.wantStatus {
				t.Fatalf("got %v, want %v", got.Status, tt.wantStatus)
			}
		})
	}
}

func Test_compareRow_Directives_Regexp(t *testing.T) {
	type args struct {
		offset   int
		expected []Value
		actual   []Value
	}

	tests := []struct {
		name       string
		args       args
		wantStatus AssertStatus
	}{
		{
			name: "regexp match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "email", Value: []any{"regexp", "^.+@example\\.com$"}}},
				actual:   []Value{{Key: "email", Value: "foo@example.com"}},
			},
			wantStatus: Match,
		},
		{
			name: "regexp not match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "email", Value: []any{"regexp", "^.+@example\\.com$"}}},
				actual:   []Value{{Key: "email", Value: "bar@other.com"}},
			},
			wantStatus: NotMatch,
		},
		{
			name: "regexp invalid pattern => not match",
			args: args{
				offset:   0,
				expected: []Value{{Key: "email", Value: []any{"regexp", "(*invalid"}}},
				actual:   []Value{{Key: "email", Value: "foo@example.com"}},
			},
			wantStatus: NotMatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareRow(tt.args.offset, tt.args.expected, tt.args.actual)
			if got.Status != tt.wantStatus {
				t.Fatalf("got %v, want %v", got.Status, tt.wantStatus)
			}
		})
	}
}
