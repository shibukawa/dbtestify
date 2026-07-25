package dbtestify

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
)

var okC = color.New(color.FgGreen).SprintfFunc()
var pkeyL = color.New(color.FgBlue, color.Bold, color.Underline).SprintfFunc()
var pkeyV = color.New(color.FgBlue, color.Underline).SprintfFunc()
var actualLC = color.New(color.FgRed).SprintfFunc()
var actualTC = color.New(color.BgRed, color.FgBlack).SprintfFunc()
var expectLC = color.New(color.FgGreen).SprintfFunc()
var expectTC = color.New(color.BgGreen, color.FgBlack).SprintfFunc()
var nameC = color.New(color.FgBlue, color.Bold).SprintfFunc()

// DiffFormat controls how an AssertTableResult is rendered.
type DiffFormat struct {
	ShowTableName bool // print a "Table: name" header
	Quiet         bool // print nothing for a matching table
	NoColor       bool // render plain text without ANSI escapes
}

type palette struct {
	ok      func(string, ...any) string
	pkeyL   func(string, ...any) string
	pkeyV   func(string, ...any) string
	actualL func(string, ...any) string
	actualT func(string, ...any) string
	expectL func(string, ...any) string
	expectT func(string, ...any) string
	name    func(string, ...any) string
}

func newPalette(noColor bool) palette {
	if noColor {
		return palette{
			ok: fmt.Sprintf, pkeyL: fmt.Sprintf, pkeyV: fmt.Sprintf,
			actualL: fmt.Sprintf, actualT: fmt.Sprintf,
			expectL: fmt.Sprintf, expectT: fmt.Sprintf,
			name:    fmt.Sprintf,
		}
	}
	return palette{
		ok: okC, pkeyL: pkeyL, pkeyV: pkeyV,
		actualL: actualLC, actualT: actualTC,
		expectL: expectLC, expectT: expectTC,
		name:    nameC,
	}
}

// FormatTableDiff renders one AssertTableResult as text.
//
// It returns an empty string when the table matches and format.Quiet is set
// without format.ShowTableName. Use format.NoColor to obtain plain text
// suitable for a test log or an error message.
func FormatTableDiff(result AssertTableResult, format DiffFormat) string {
	var b strings.Builder
	writeTableDiff(&b, result, format)
	return b.String()
}

// DumpDiffCallback returns an AssertOpt.DiffCallback that writes each table
// result to w.
func DumpDiffCallback(w io.Writer, format DiffFormat) func(result AssertTableResult) {
	return func(result AssertTableResult) {
		writeTableDiff(w, result, format)
	}
}

// DumpDiffCLICallback returns an AssertOpt.DiffCallback that writes colored
// output to os.Stdout.
//
// It is equivalent to DumpDiffCallback(os.Stdout, DiffFormat{ShowTableName: showTableName, Quiet: quiet}).
func DumpDiffCLICallback(showTableName, quiet bool) func(result AssertTableResult) {
	return DumpDiffCallback(os.Stdout, DiffFormat{ShowTableName: showTableName, Quiet: quiet})
}

func writeTableDiff(w io.Writer, result AssertTableResult, format DiffFormat) {
	if result.Status == Match && format.Quiet && !format.ShowTableName {
		return
	}
	p := newPalette(format.NoColor)
	if format.ShowTableName {
		fmt.Fprint(w, p.name("Table: %s\n", result.Name))
	}
	if result.Status == Match {
		if !format.Quiet {
			fmt.Fprintf(w, " %s\n", p.ok("OK"))
		}
		return
	}
	fmt.Fprint(w, p.expectL("- Expected\n"))
	fmt.Fprint(w, p.actualL("+ Actual\n"))

	for _, r := range result.Rows {
		for i := range result.PrimaryKeys {
			fmt.Fprint(w, p.pkeyL("%s", r.Fields[i].Key))
			if r.Status == OnlyOnActual {
				fmt.Fprint(w, p.pkeyV(": %v", r.Fields[i].Actual))
			} else {
				fmt.Fprint(w, p.pkeyV(": %v", r.Fields[i].Expect))
			}
			if i+1 == len(result.PrimaryKeys) {
				fmt.Fprint(w, "\n")
			} else {
				fmt.Fprint(w, p.pkeyV(", "))
			}
		}
		switch r.Status {
		case Match:
			fmt.Fprint(w, "  ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprint(w, ", ")
				}
				fmt.Fprintf(w, "%s: %v", f.Key, f.Expect)
			}
		case NotMatch:
			fmt.Fprint(w, p.expectT("+")+" ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprint(w, p.expectL(", "))
				}
				if f.Status == Match {
					fmt.Fprint(w, p.expectL("%s: %v", f.Key, f.Expect))
				} else {
					fmt.Fprint(w, p.expectL("%s: ", f.Key))
					e := fmt.Sprintf("%v", f.Expect)
					a := fmt.Sprintf("%v", f.Actual)
					fmt.Fprint(w, p.expectT("%s", e))
					fmt.Fprint(w, strings.Repeat(" ", max(len(a)-len(e), 0)))
				}
			}
			fmt.Fprint(w, "\n"+p.actualT("-")+" ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprint(w, p.actualL(", "))
				}
				if f.Status == Match {
					fmt.Fprint(w, p.actualL("%s: %v", f.Key, f.Actual))
				} else {
					fmt.Fprint(w, p.actualL("%s: ", f.Key))
					e := fmt.Sprintf("%v", f.Expect)
					a := fmt.Sprintf("%v", f.Actual)
					fmt.Fprint(w, p.actualT("%v", f.Actual))
					fmt.Fprint(w, strings.Repeat(" ", max(len(e)-len(a), 0)))
				}
			}
		case OnlyOnExpect:
			fmt.Fprint(w, p.expectT("+")+" ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprint(w, p.expectL(", "))
				}
				fmt.Fprint(w, p.expectT("%s: %v", f.Key, f.Expect))
			}
		case OnlyOnActual:
			fmt.Fprint(w, p.actualT("-")+" ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprint(w, p.actualL(", "))
				}
				fmt.Fprint(w, p.actualT("%s: %v", f.Key, f.Actual))
			}
		}
		fmt.Fprint(w, "\n")
	}
	fmt.Fprint(w, "\n")
}
