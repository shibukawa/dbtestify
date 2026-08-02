package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/shibukawa/dbtestify"
)

type AssertOpt struct {
	IncludeTags []string `json:"include_tags"`
	ExcludeTags []string `json:"exclude_tags"`
	Targets     []string `json:"targets"`
}

type AssertTableResult struct {
	Table       string              `json:"table"`
	PrimaryKeys []string            `json:"primary_keys"`
	Match       bool                `json:"match"`
	Diff        []dbtestify.RowDiff `json:"diff"`
	Error       string              `json:"error,omitzero"`
}

type AssertResponse struct {
	Tables []AssertTableResult `json:"tables"`
}

func assertTable(ctx context.Context, dbc dbtestify.DBConnector, useJson bool, w http.ResponseWriter, dataSets fs.FS, path string, reqOpt AssertOpt) (bool, error) {
	f, err := dataSets.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	data, err := dbtestify.ParseYAML(f)
	if err != nil {
		return false, err
	}

	ok, cResult, err := dbtestify.Assert(ctx, dbc, data, dbtestify.AssertOpt{
		IncludeTags:  reqOpt.IncludeTags,
		ExcludeTags:  reqOpt.ExcludeTags,
		TargetTables: reqOpt.Targets,
	})
	if err != nil {
		return false, err
	}
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
	}
	if useJson {
		var result AssertResponse
		for _, tr := range cResult {
			result.Tables = append(result.Tables, AssertTableResult{
				Table:       tr.Name,
				PrimaryKeys: tr.PrimaryKeys,
				Match:       tr.Status == dbtestify.Match,
				Diff:        tr.Rows,
			})
		}
		e := json.NewEncoder(w)
		e.Encode(&result)
	} else {
		for _, tr := range cResult {
			dumpDiff(w, tr)
		}
	}
	return ok, nil
}

func dumpDiff(w io.Writer, result dbtestify.AssertTableResult) {
	fmt.Fprintf(w, "🗄️ '%s' table\n", result.Name)
	fmt.Fprintf(w, "🟢 Expected\n")
	fmt.Fprintf(w, "🟥 Actual\n\n")

	for _, r := range result.Rows {
		for i := range result.PrimaryKeys {
			fmt.Fprintf(w, "%s: %v", r.Fields[i].Key, r.Fields[i].Expect)
			if i+1 == len(result.PrimaryKeys) {
				fmt.Fprintf(w, "\n")
			} else {
				fmt.Fprintf(w, ", ")
			}
		}
		switch r.Status {
		case dbtestify.Match:
			fmt.Fprintf(w, "   ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprintf(w, ", ")
				}
				fmt.Fprintf(w, "%s: %v", f.Key, f.Expect)
			}
		case dbtestify.NotMatch:
			fmt.Fprintf(w, "🟢 ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprintf(w, ", ")
				}
				if f.Status == dbtestify.Match {
					fmt.Fprintf(w, "%s: %v", f.Key, f.Expect)
				} else {
					fmt.Fprintf(w, "%s: ", f.Key)
					e := fmt.Sprintf("%v", f.Expect)
					a := fmt.Sprintf("%v", f.Actual)
					fmt.Fprint(w, e)
					fmt.Fprint(w, strings.Repeat(" ", max(len(a)-len(e), 0)))
				}
			}
			fmt.Fprintf(w, "\n🟥 ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprintf(w, ", ")
				}
				if f.Status == dbtestify.Match {
					fmt.Fprintf(w, "%s: %v", f.Key, f.Actual)
				} else {
					fmt.Fprintf(w, "%s: ", f.Key)
					e := fmt.Sprintf("%v", f.Expect)
					a := fmt.Sprintf("%v", f.Actual)
					fmt.Fprintf(w, "%v", f.Actual)
					fmt.Fprint(w, strings.Repeat(" ", max(len(e)-len(a), 0)))
				}
			}
		case dbtestify.OnlyOnActual:
			fmt.Fprintf(w, "\n🟥 ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprintf(w, ", ")
				}
				fmt.Fprintf(w, "%s: %v", f.Key, f.Actual)
			}
		case dbtestify.OnlyOnExpect:
			fmt.Fprintf(w, "🟢 ")
			for i, f := range r.Fields {
				if i < len(result.PrimaryKeys) {
					continue
				}
				if i != len(result.PrimaryKeys) {
					fmt.Fprintf(w, ", ")
				}
				fmt.Fprintf(w, "%s: %v", f.Key, f.Expect)
			}
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "\n")
}
