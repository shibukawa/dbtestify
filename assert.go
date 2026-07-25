package dbtestify

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// AssertResult represents the result of an assertion operation on a dataset.
type AssertResult struct {
	Tables []AssertTableResult
}

// AssertStatus defines the status of an assertion result.
type AssertStatus string

const (
	Match        AssertStatus = "match"
	NotMatch     AssertStatus = "not-match"
	OnlyOnExpect AssertStatus = "only-e"
	OnlyOnActual AssertStatus = "only-a"
	WrongDataSet AssertStatus = "wrongDataSet" // primary keys are missing
)

// AssertTableResult represents the result of an assertion on a single table in AssertResult.
type AssertTableResult struct {
	Name        string
	PrimaryKeys []string
	Rows        []RowDiff
	Status      AssertStatus
}

// RowDiff represents the difference in a row between the expected and actual data.
type RowDiff struct {
	Fields []Diff       `json:"fields"`
	Status AssertStatus `json:"status"`
}

// Value represents a single value in a row, including its key and value.
type Diff struct {
	Key    string       `json:"key"`
	Expect any          `json:"expect"`
	Actual any          `json:"actual"`
	Status AssertStatus `json:"status"`
}

// MatchStrategy defines the strategy for matching rows in a table.
type AssertOpt struct {
	IncludeTags  []string                                                            // Tags to filter rows of dataset.
	ExcludeTags  []string                                                            // Tags to filter rows of dataset.
	TargetTables []string                                                            // Only specified tables will be processed. If empty, all tables will be processed.
	Callback     func(targetTable string, mode MatchStrategy, start bool, err error) // Callback function to report progress and errors during the assertion process.
	DiffCallback func(result AssertTableResult)                                      // Callback function to report differences in rows during the assertion process.
}

// Assert performs an assertion on the provided dataset against the database.
func Assert(ctx context.Context, dbc DBConnector, expected *DataSet, opt AssertOpt) (bool, []AssertTableResult, error) {
	var errs []error
	var result []AssertTableResult
	ok := true
	for _, t := range expected.Tables {
		if len(opt.TargetTables) > 0 {
			if !slices.Contains(opt.TargetTables, t.Name) {
				continue
			}
		}
		strategy := ExactMatchStrategy
		if s, ok := expected.Match[t.Name]; ok {
			strategy = s
		}
		if opt.Callback != nil {
			opt.Callback(t.Name, strategy, true, nil)
		}
		actual, sortKeys, err := fetchTableData(ctx, dbc, t.Name)
		if opt.Callback != nil {
			opt.Callback(t.Name, strategy, false, err)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		expectedNormalizedTable, err := t.SortAndFilter(sortKeys, opt.IncludeTags, opt.ExcludeTags)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		r := compareTable(t.Name, strategy, sortKeys, expectedNormalizedTable.Rows, actual)
		result = append(result, r)
		if r.Status == NotMatch {
			ok = false
		}
		if opt.DiffCallback != nil {
			opt.DiffCallback(r)
		}
	}
	if len(errs) > 0 {
		return false, nil, errors.Join(errs...)
	}
	return ok, result, nil
}

func compareTable(tableName string, strategy MatchStrategy, pKeys []string, expected, actual [][]Value) AssertTableResult {
	result := AssertTableResult{
		Name:        tableName,
		PrimaryKeys: pKeys,
	}
	var i, j int
	ok := true
	for i < len(expected) && j < len(actual) {
		e := expected[i]
		a := actual[j]
		cr := comparePkey(len(pKeys), e, a)
		if cr == 0 { // keys equal
			i++
			j++
			row := compareRow(len(pKeys), e, a)
			if row.Status != Match {
				ok = false
			}
			result.Rows = append(result.Rows, row)
			continue
		}
		if cr < 0 { // only on expected
			i++
			row := make([]Diff, len(e))
			for ii, f := range e {
				row[ii] = Diff{Key: f.Key, Expect: f.Value}
			}
			result.Rows = append(result.Rows, RowDiff{
				Fields: row,
				Status: OnlyOnExpect,
			})
			ok = false
			continue
		}
		// cr > 0 : only on actual
		j++
		if strategy == ExactMatchStrategy {
			row := make([]Diff, len(a))
			for ii, f := range a {
				row[ii] = Diff{Key: f.Key, Actual: f.Value}
			}
			result.Rows = append(result.Rows, RowDiff{
				Fields: row,
				Status: OnlyOnActual,
			})
			ok = false
		}
	}
	for i < len(expected) {
		e := expected[i]
		i++
		row := make([]Diff, len(e))
		for i, f := range e {
			row[i] = Diff{Key: f.Key, Expect: f.Value}
		}
		result.Rows = append(result.Rows, RowDiff{
			Fields: row,
			Status: OnlyOnExpect,
		})
		ok = false
	}
	for j < len(actual) {
		a := actual[j]
		j++
		if strategy == ExactMatchStrategy {
			row := make([]Diff, len(a))
			for i, f := range a {
				row[i] = Diff{Key: f.Key, Actual: f.Value}
			}
			result.Rows = append(result.Rows, RowDiff{
				Fields: row,
				Status: OnlyOnActual,
			})
			ok = false
		}
	}
	if ok {
		result.Status = Match
	} else {
		result.Status = NotMatch
	}
	return result
}

func comparePkey(pkeyCount int, key1, key2 []Value) int {
	for i := range pkeyCount {
		v1 := key1[i].Value
		v2 := key2[i].Value
		c := -1
		switch v1t := v1.(type) { // todo other types
		case int:
			if v2t, ok := v2.(int); !ok {
				break
			} else {
				c = cmp.Compare(v1t, v2t)
			}
		case float64:
			if v2t, ok := v2.(float64); !ok {
				break
			} else {
				c = cmp.Compare(v1t, v2t)
			}
		case string:
			if v2t, ok := v2.(string); !ok {
				break
			} else {
				c = cmp.Compare(v1t, v2t)
			}
		}
		if c == -1 { // can't convert to primitive
			c = cmp.Compare(fmt.Sprint(v1), fmt.Sprint(v2))
		}
		if c != 0 { // check next primary key
			return c
		}
	}
	return 0
}

// compareRow compares Row
//
// Input filed data should be sorted with same order.
func compareRow(offset int, expected, actual []Value) RowDiff {
	result := make([]Diff, offset, len(expected))
	// Store Primary Key fields
	for o := range offset {
		e := expected[o]
		a := actual[o]
		result[o] = Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match}
	}
	// Test other fields
	i := offset
	j := offset
	allOk := true
	for i < len(expected) && j < len(actual) {
		e := expected[i]
		a := actual[j]
		if e.Key == a.Key {
			i++
			j++
			if s, ok := e.Value.([]any); ok {
				switch s[0] {
				case "null":
					fallthrough
				case nil:
					if a.Value == nil {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match})
					} else {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
						allOk = false
					}
				case "notnull":
					if a.Value != nil {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match})
					} else {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
						allOk = false
					}
				case "any":
					result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match})
				case "currentdate":
					// [currentdate, duration] duration default: 1m
					dur := time.Minute
					if len(s) >= 2 && s[1] != nil {
						if dstr, ok := s[1].(string); ok {
							if parsed, err := time.ParseDuration(dstr); err == nil {
								dur = parsed
							}
						} else {
							if parsed, err := time.ParseDuration(fmt.Sprint(s[1])); err == nil {
								dur = parsed
							}
						}
					}
					now := timeNow()
					var at time.Time
					ok := false
					switch av := a.Value.(type) {
					case time.Time:
						at = av
						ok = true
					case string:
						if t, parsed := parseTimeLoose(av); parsed {
							at = t
							ok = true
						}
					}
					if ok {
						if absDuration(now.Sub(at)) <= dur {
							result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match})
						} else {
							result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
							allOk = false
						}
					} else {
						// unsupported actual type or parse failure
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
						allOk = false
					}
				case "regexp":
					// [regexp, pattern]
					if len(s) < 2 || s[1] == nil {
						// No pattern provided -> treat as not match
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
						allOk = false
						break
					}
					pattern := fmt.Sprint(s[1])
					re, err := regexp.Compile(pattern)
					if err != nil {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
						allOk = false
						break
					}
					var target string
					if a.Value == nil {
						target = ""
					} else {
						target = fmt.Sprint(a.Value)
					}
					if re.MatchString(target) {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match})
					} else {
						result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
						allOk = false
					}
				default:
					panic("not implemented: [" + s[0].(string) + "]")
				}
			} else if e.Value == a.Value {
				result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: Match})
			} else {
				result = append(result, Diff{Key: e.Key, Expect: e.Value, Actual: a.Value, Status: NotMatch})
				allOk = false
			}
		} else if e.Key > a.Key { // field only in actual row is ignored. You can omit system column in data set
			j++
		} else {
			i++
			result = append(result, Diff{Key: e.Key, Expect: e.Value, Status: WrongDataSet})
			allOk = false
		}
	}
	for i < len(expected) {
		e := expected[i]
		i++
		result = append(result, Diff{Key: e.Key, Expect: e.Value, Status: WrongDataSet})
		allOk = false
	}
	for j < len(actual) {
		j++
	}
	if allOk {
		return RowDiff{
			Fields: result,
			Status: Match,
		}
	}
	return RowDiff{
		Fields: result,
		Status: NotMatch,
	}
}

func fetchTableData(ctx context.Context, dbc DBConnector, tableName string) ([][]Value, []string, error) {
	pkeys, err := dbc.PrimaryKeys(ctx, tableName)
	if err != nil {
		return nil, nil, err
	}

	rows, err := dbc.Executor().QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", tableName))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query table %s: %w", tableName, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var result [][]Value
	for rows.Next() {
		row := make(map[string]any)
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, nil, fmt.Errorf("failed to scan row: %w", err)
		}

		for i, colName := range columns {
			val := values[i]
			if val != nil {
				switch val2 := val.(type) {
				case []byte:
					row[colName] = string(val2)
				case int64:
					row[colName] = int(val2)
				default:
					row[colName] = val2
				}
			} else {
				row[colName] = nil
			}
		}
		sliceRow, _ := mapToValues(row, pkeys)
		result = append(result, sliceRow)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("row iteration error: %w", err)
	}

	sortRow(result, pkeys)

	return result, pkeys, nil
}

// timeNow is a var to allow tests to stub current time.
var timeNow = time.Now

// absDuration returns absolute value of a duration.
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// parseTimeLoose tries several common layouts to parse time string.
func parseTimeLoose(s string) (time.Time, bool) {
	// Try RFC3339 and RFC3339Nano first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	// Common SQL timestamp without timezone
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t, true
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", s, time.Local); err == nil {
		return t, true
	}
	// Date only
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, true
	}
	return time.Time{}, false
}
