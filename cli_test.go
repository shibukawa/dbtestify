package dbtestify

import (
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func notMatchResult() AssertTableResult {
	return AssertTableResult{
		Name:        "member",
		PrimaryKeys: []string{"id"},
		Status:      NotMatch,
		Rows: []RowDiff{{
			Status: NotMatch,
			Fields: []Diff{
				{Key: "id", Expect: 1, Actual: 1, Status: Match},
				{Key: "name", Expect: "Frank", Actual: "Grace", Status: NotMatch},
			},
		}},
	}
}

func TestFormatTableDiffNoColor(t *testing.T) {
	got := FormatTableDiff(notMatchResult(), DiffFormat{ShowTableName: true, NoColor: true})

	assert.False(t, strings.Contains(got, "\x1b["), "plain output must not contain ANSI escapes: %q", got)
	assert.Contains(t, got, "Table: member")
	assert.Contains(t, got, "- Expected")
	assert.Contains(t, got, "+ Actual")
	assert.Contains(t, got, "Frank")
	assert.Contains(t, got, "Grace")
}

func TestFormatTableDiffQuietMatch(t *testing.T) {
	match := AssertTableResult{Name: "member", PrimaryKeys: []string{"id"}, Status: Match}

	assert.Equal(t, "", FormatTableDiff(match, DiffFormat{Quiet: true, NoColor: true}))
	assert.Contains(t, FormatTableDiff(match, DiffFormat{NoColor: true}), "OK")
}

// TestFormatTableDiffPercentValue covers values containing a verb-like token,
// which must survive formatting verbatim.
func TestFormatTableDiffPercentValue(t *testing.T) {
	result := notMatchResult()
	result.Rows[0].Fields[1].Expect = "100%s"

	got := FormatTableDiff(result, DiffFormat{NoColor: true})

	assert.Contains(t, got, "100%s")
	assert.False(t, strings.Contains(got, "%!"), "unexpected formatting error in: %q", got)
}

func TestDumpDiffCallbackWritesToWriter(t *testing.T) {
	var b strings.Builder
	DumpDiffCallback(&b, DiffFormat{ShowTableName: true, Quiet: true, NoColor: true})(notMatchResult())

	assert.Contains(t, b.String(), "Table: member")
	assert.Contains(t, b.String(), "Grace")
}
