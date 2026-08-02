package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

const MAX_MEMORY_BYTES = 1024 * 1024

func jsonAcceptable(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func parseSeedRequest(r *http.Request) (*SeedOpt, error) {
	contentType := r.Header.Get("Content-Type")
	var opt SeedOpt
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		decoder := json.NewDecoder(r.Body)
		defer r.Body.Close()
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&opt)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		fallthrough
	case strings.HasPrefix(contentType, "multipart/form-data"):
		if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
			err := r.ParseForm()
			if err != nil {
				return nil, err
			}
		} else {
			err := r.ParseMultipartForm(MAX_MEMORY_BYTES)
			if err != nil {
				return nil, err
			}
		}
		opt.IncludeTags = append(r.Form["i"], r.Form["include_tag"]...)
		opt.ExcludeTags = append(r.Form["e"], r.Form["exclude_tag"]...)
		opt.Targets = append(r.Form["t"], r.Form["target"]...)
		opt.Truncates = r.Form["truncate"]
		if batchSize := r.Form.Get("batch_size"); batchSize != "" {
			batchSizeInt, err := strconv.Atoi(batchSize)
			if err != nil {
				return nil, err
			}
			opt.BatchSize = batchSizeInt
		}
	}
	slices.Sort(opt.IncludeTags)
	slices.Sort(opt.ExcludeTags)
	slices.Sort(opt.Targets)
	slices.Sort(opt.Truncates)
	if opt.BatchSize == 0 {
		opt.BatchSize = 50
	}
	return &opt, nil
}

func parseAssertRequest(r *http.Request) AssertOpt {
	params := r.URL.Query()
	opt := AssertOpt{
		IncludeTags: append(params["i"], params["include-tag"]...),
		ExcludeTags: append(params["e"], params["exclude-tag"]...),
		Targets:     append(params["t"], params["target"]...),
	}
	slices.Sort(opt.IncludeTags)
	slices.Sort(opt.ExcludeTags)
	slices.Sort(opt.Targets)
	return opt
}
