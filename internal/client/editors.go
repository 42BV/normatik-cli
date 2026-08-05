package client

import (
	"context"
	"net/http"
	"strconv"

	"github.com/42BV/normatik-cli/internal/api"
)

// pageableEditor sets page/size/sort as individual query params. This bypasses
// the generated Pageable object-styling entirely (its `Sort *[]string` field
// breaks the oapi-codegen runtime). Pass the Pageable struct empty and let this
// editor own page/size/sort.
func pageableEditor(page, size int, sort []string) api.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		q := req.URL.Query()
		if page > 0 {
			q.Set("page", strconv.Itoa(page))
		}
		if size > 0 {
			q.Set("size", strconv.Itoa(size))
		}
		for _, s := range sort {
			if s != "" {
				q.Add("sort", s)
			}
		}
		req.URL.RawQuery = q.Encode()
		return nil
	}
}

// expandEditor sets expand sections as a query param, bypassing the per-endpoint
// typed enum cast (FindById2ParamsExpand etc.). The server reads a comma-joined
// or repeated `expand` param identically.
func expandEditor(sections []string) api.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		q := req.URL.Query()
		for _, s := range sections {
			if s != "" {
				q.Add("expand", s)
			}
		}
		req.URL.RawQuery = q.Encode()
		return nil
	}
}
