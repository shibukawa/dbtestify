package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestParseSeedRequest(t *testing.T) {
	tests := []struct {
		name          string
		createRequest func() *http.Request
		expected      SeedOpt
	}{
		{
			name: "no param",
			createRequest: func() *http.Request {
				req, _ := http.NewRequest("POST", "/api/seed/test.yaml", nil)
				return req
			},
			expected: SeedOpt{
				BatchSize: 50,
			},
		},
		{
			name: "json",
			createRequest: func() *http.Request {
				req, _ := http.NewRequest("POST", "/api/seed/test.yaml", strings.NewReader(`{
					"include_tags": ["a", "b"],
					"exclude_tags": ["c", "d"],
					"targets": ["e", "f"],
					"truncates": ["g", "h"],
					"batch_size": 100
				}`))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expected: SeedOpt{
				IncludeTags: []string{"a", "b"},
				ExcludeTags: []string{"c", "d"},
				Targets:     []string{"e", "f"},
				Truncates:   []string{"g", "h"},
				BatchSize:   100,
			},
		},
		{
			name: "from",
			createRequest: func() *http.Request {
				body := url.Values{}
				body.Add("include_tag", "a")
				body.Add("i", "b")
				body.Add("exclude_tag", "c")
				body.Add("e", "d")
				body.Add("target", "e")
				body.Add("t", "f")
				body.Add("truncate", "g")
				body.Add("truncate", "h")
				body.Add("batch_size", "100")
				req, _ := http.NewRequest("POST", "/api/seed/test.yaml", strings.NewReader(body.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req
			},
			expected: SeedOpt{
				IncludeTags: []string{"a", "b"},
				ExcludeTags: []string{"c", "d"},
				Targets:     []string{"e", "f"},
				Truncates:   []string{"g", "h"},
				BatchSize:   100,
			},
		},
		{
			name: "multipart",
			createRequest: func() *http.Request {
				var requestBody bytes.Buffer
				writer := multipart.NewWriter(&requestBody)
				writer.WriteField("include_tag", "a")
				writer.WriteField("i", "b")
				writer.WriteField("exclude_tag", "c")
				writer.WriteField("e", "d")
				writer.WriteField("target", "e")
				writer.WriteField("t", "f")
				writer.WriteField("truncate", "g")
				writer.WriteField("truncate", "h")
				writer.WriteField("batch_size", "100")
				writer.Close()
				req, _ := http.NewRequest("POST", "/api/seed/test.yaml", &requestBody)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				return req
			},
			expected: SeedOpt{
				IncludeTags: []string{"a", "b"},
				ExcludeTags: []string{"c", "d"},
				Targets:     []string{"e", "f"},
				Truncates:   []string{"g", "h"},
				BatchSize:   100,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.createRequest()
			got, err := parseSeedRequest(req)
			assert.NoError(t, err)
			if !reflect.DeepEqual(*got, tt.expected) {
				t.Errorf("ParseAssertRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseAssertRequest(t *testing.T) {
	tests := []struct {
		name          string
		createRequest func() *http.Request
		expected      AssertOpt
	}{
		{
			name: "no param",
			createRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/api/assert/test.yaml", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expected: AssertOpt{},
		},
		{
			name: "include tag",
			createRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/api/assert/test.yaml?include-tag=a&i=b", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expected: AssertOpt{
				IncludeTags: []string{"a", "b"},
			},
		},
		{
			name: "exclude tag",
			createRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/api/assert/test.yaml?exclude-tag=a&e=b", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expected: AssertOpt{
				ExcludeTags: []string{"a", "b"},
			},
		},
		{
			name: "target",
			createRequest: func() *http.Request {
				req, _ := http.NewRequest("GET", "/api/assert/test.yaml?target=a&t=b", nil)
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expected: AssertOpt{
				Targets: []string{"a", "b"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.createRequest()
			got := parseAssertRequest(req)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ParseAssertRequest() = %v, want %v", got, tt.expected)
			}
		})
	}
}
