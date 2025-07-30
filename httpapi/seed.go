package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/shibukawa/dbtestify"
)

type SeedOpt struct {
	IncludeTags []string `json:"include_tags"`
	ExcludeTags []string `json:"exclude_tags"`
	BatchSize   int      `json:"batch_size"`
	Truncates   []string `json:"truncates"`
	Targets     []string `json:"targets"`
}

type SeedTableResult struct {
	Task     string        `json:"task"`
	Table    string        `json:"table"`
	Success  bool          `json:"success"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitzero"`
}

type SeedResponse struct {
	Tables []SeedTableResult `json:"tables"`
}

func seedTable(ctx context.Context, dbc dbtestify.DBConnector, useJson bool, w io.Writer, path string, reqOpt SeedOpt) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := dbtestify.ParseYAML(f)
	if err != nil {
		return err
	}

	var startTime time.Time
	var result SeedResponse
	opt := dbtestify.SeedOpt{
		BatchSize:    reqOpt.BatchSize,
		Operations:   data.Operation,
		IncludeTags:  reqOpt.IncludeTags,
		ExcludeTags:  reqOpt.ExcludeTags,
		TargetTables: reqOpt.Targets,
		Callback: func(targetTable, task string, start bool, err error) {
			if start {
				startTime = time.Now()
			} else if err != nil {
				result.Tables = append(result.Tables, SeedTableResult{
					Task:     task,
					Table:    targetTable,
					Success:  false,
					Duration: time.Since(startTime),
					Error:    err.Error(),
				})
			} else {
				result.Tables = append(result.Tables, SeedTableResult{
					Task:     task,
					Table:    targetTable,
					Success:  true,
					Duration: time.Since(startTime),
				})
			}
		},
	}
	for _, t := range reqOpt.Truncates {
		opt.Operations[t] = dbtestify.TruncateOperation
	}
	err = dbtestify.Seed(ctx, dbc, data, opt)
	if err != nil {
		return err
	}
	if useJson {
		e := json.NewEncoder(w)
		e.Encode(&result)
	} else {
		for _, t := range result.Tables {
			fmt.Fprintf(w, "%s '%s' table -> ", t.Task, t.Table)
			if t.Success {
				fmt.Fprintf(w, "ok (%v)\n", t.Duration)
			} else {
				fmt.Fprintf(w, "error\n    %s\n", t.Error)
			}
		}
	}
	return nil
}
