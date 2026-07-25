package dbtestify

import (
	"context"
	"errors"
	"maps"
	"slices"
)

// DefaultBatchSize is the default number of rows to process in a single batch during seeding.
var DefaultBatchSize = 50

// SeedOpt defines options for the seeding process.
type SeedOpt struct {
	BatchSize    int                                                   // default: 50
	Operations   map[string]Operation                                  // Operations to apply to each table. If empty, defaults to ClearInsertOperation.
	IncludeTags  []string                                              // Tags to filter rows of dataset.
	ExcludeTags  []string                                              // Tags to filter rows of dataset.
	TargetTables []string                                              // Only specified tables will be processed.
	Callback     func(targetTable, task string, start bool, err error) // Callback function to report progress and errors during the seeding process.
}

// Seed initializes the database with the provided dataset, applying the specified operations.
//
// A connector built from a pool runs the whole dataset in one transaction that
// Seed opens and commits. A connector built with [NewDBConnectorFromTx] runs in
// the caller's transaction instead, and Seed neither commits nor rolls back.
func Seed(ctx context.Context, dbc DBConnector, data *DataSet, opt SeedOpt) error {
	if opt.BatchSize == 0 {
		opt.BatchSize = DefaultBatchSize
	}
	if dbc.InTransaction() {
		return seedInto(ctx, dbc, dbc.Executor(), data, opt)
	}
	db := dbc.DB()
	if db == nil {
		return errors.New("dbtestify: connector has no pool to open a transaction on")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := seedInto(ctx, dbc, tx, data, opt); err != nil {
		return err
	}
	return tx.Commit()
}

func seedInto(ctx context.Context, dbc DBConnector, exec Executor, data *DataSet, opt SeedOpt) error {
	// truncate first
	ops := map[string]Operation{}
	for t, op := range opt.Operations {
		ops[t] = op
	}
	for _, t := range data.Tables {
		if len(opt.TargetTables) > 0 {
			if !slices.Contains(opt.TargetTables, t.Name) {
				continue
			}
		}
		switch opt.Operations[t.Name] {
		case ClearInsertOperation:
			fallthrough
		case "":
			fallthrough
		case TruncateOperation:
			ops[t.Name] = TruncateOperation
		}
	}
	for t, op := range ops {
		if op == TruncateOperation {
			if opt.Callback != nil {
				opt.Callback(t, "truncate", true, nil)
			}
			err := dbc.Truncate(ctx, exec, t)
			if opt.Callback != nil {
				opt.Callback(t, "truncate", false, err)
			}
			if err != nil {
				return err
			}
		}
	}
	for _, t := range data.Tables {
		if len(opt.TargetTables) > 0 {
			if !slices.Contains(opt.TargetTables, t.Name) {
				continue
			}
		}
		switch opt.Operations[t.Name] {
		case ClearInsertOperation:
			fallthrough
		case "":
			fallthrough
		case InsertOperation:
			if opt.Callback != nil {
				opt.Callback(t.Name, "insert", true, nil)
			}
			err := processInsertOperation(ctx, dbc, exec, t, opt, false)
			if opt.Callback != nil {
				opt.Callback(t.Name, "insert", false, nil)
			}
			if err != nil {
				return err
			}
		case UpsertOperation:
			if opt.Callback != nil {
				opt.Callback(t.Name, "upsert", true, nil)
			}
			err := processInsertOperation(ctx, dbc, exec, t, opt, true)
			if opt.Callback != nil {
				opt.Callback(t.Name, "upsert", false, nil)
			}
			if err != nil {
				return err
			}
		case DeleteOperation:
			err := processDeleteOperation(ctx, dbc, exec, t, opt)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func processInsertOperation(ctx context.Context, dbc DBConnector, exec Executor, t *Table, opt SeedOpt, upsert bool) error {
	var pKeys []string
	if upsert {
		var err error
		pKeys, err = dbc.PrimaryKeys(ctx, t.Name)
		if err != nil {
			return err
		}

	}

	for i := 0; i < len(t.Rows); i += opt.BatchSize {
		end := i + opt.BatchSize
		if end > len(t.Rows) {
			end = len(t.Rows)
		}
		batch := t.Rows[i:end]
		columnMaps := map[string]bool{}
		for _, r := range batch {
			for k := range maps.Keys(r) {
				columnMaps[k] = true
			}
		}
		columns := slices.Sorted(maps.Keys(columnMaps))
		values := make([]any, 0, len(batch)*len(columns))
		for j, r := range batch {
			if filter(t.Tags[i+j], opt.IncludeTags, opt.ExcludeTags) {
				for _, c := range columns {
					if val, ok := r[c]; ok {
						values = append(values, val)
					} else {
						values = append(values, nil)
					}
				}
			}
		}
		if upsert {
			if err := dbc.Upsert(ctx, exec, t.Name, columns, pKeys, values); err != nil {
				return err
			}
		} else if err := dbc.Insert(ctx, exec, t.Name, columns, values); err != nil {
			return err
		}
	}
	return nil
}

func processDeleteOperation(ctx context.Context, dbc DBConnector, exec Executor, t *Table, opt SeedOpt) error {
	columns, err := dbc.PrimaryKeys(ctx, t.Name)
	if err != nil {
		return err
	}
	for i := 0; i < len(t.Rows); i += opt.BatchSize {
		end := i + opt.BatchSize
		if end > len(t.Rows) {
			end = len(t.Rows)
		}
		batch := t.Rows[i:end]
		values := make([]any, 0, len(batch)*len(columns))
		for j, r := range batch {
			if filter(t.Tags[i+j], opt.IncludeTags, opt.ExcludeTags) {
				for _, c := range columns {
					if val, ok := r[c]; ok {
						values = append(values, val)
					} else {
						values = append(values, nil)
					}
				}
			}
		}
		if err := dbc.Delete(ctx, exec, t.Name, columns, values); err != nil {
			return err
		}
	}
	return nil
}
