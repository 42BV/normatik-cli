// Package client is the hand-written wrapper around the generated transport.
// It owns auth, base URL, and — crucially — error decoding into Problem, which
// is where the CLI's discoverability value lives. The generated client is
// treated strictly as a transport layer; wrappers go through Do/DoRaw so the
// 2xx-check, ProblemDetail decode and `*/*`-quirk are solved in exactly one place.
package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/httpx"
	"github.com/42BV/normatik-cli/internal/problem"
)

type Client struct {
	api     *api.ClientWithResponses
	http    *http.Client
	apiBase string
	apiKey  string
}

// APIError carries either a decoded Problem or a malformed-response marker.
type APIError struct {
	Problem   *problem.Problem
	TooLarge  *ResponseTooLargeError
	Status    int
	Body      []byte
	Malformed bool // true when the body was not a recognizable ProblemDetail
	Transport error
}

func (e *APIError) Error() string {
	if e.Problem != nil {
		return e.Problem.Error()
	}
	if e.TooLarge != nil {
		return e.TooLarge.Error()
	}
	if e.Transport != nil {
		return e.Transport.Error()
	}
	return "request failed"
}

var ErrResponseTooLarge = errors.New("response too large")

type ResponseTooLargeError struct {
	Status int
	Limit  int64
}

func (e *ResponseTooLargeError) Error() string {
	return fmt.Sprintf("HTTP %d response exceeds %d-byte limit", e.Status, e.Limit)
}

func (e *ResponseTooLargeError) Unwrap() error { return ErrResponseTooLarge }

func (e *APIError) ExitCode() int {
	if e.Problem != nil {
		return e.Problem.ExitCode()
	}
	if e.Malformed {
		return 1
	}
	return 1
}

func New(baseURL, apiKey string) (*Client, error) {
	if err := httpx.ValidateBaseURL(baseURL); err != nil {
		return nil, err
	}
	// 120s (was 30s): GET /public/v1/content-macros/{name}/scan walks every page
	// server-side and crossed 30s on realistic datasets (live-caught: 26.7s green,
	// then >30s context-deadline red as the E2E suite grew). The scan has no
	// per-request override path — the generated client shares one http.Client —
	// so the generic ceiling carries the headroom.
	hc := httpx.NewClient(120 * time.Second)
	auth := func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		return nil
	}
	apiBase := httpx.APIBaseURL(baseURL)
	c, err := api.NewClientWithResponses(apiBase, api.WithHTTPClient(hc), api.WithRequestEditorFn(auth))
	if err != nil {
		return nil, err
	}
	return &Client{api: c, http: hc, apiBase: apiBase, apiKey: apiKey}, nil
}

func fail(transport error) *APIError { return &APIError{Transport: transport} }

func responseTooLarge(status int, limit int64) *APIError {
	tooLarge := &ResponseTooLargeError{Status: status, Limit: limit}
	return &APIError{Status: status, TooLarge: tooLarge}
}

// APIErrorFromRead converts a typed streaming read failure into the same
// APIError shape used for Content-Length and buffered-response limit failures.
func APIErrorFromRead(err error) *APIError {
	var tooLarge *ResponseTooLargeError
	if errors.As(err, &tooLarge) {
		return responseTooLarge(tooLarge.Status, tooLarge.Limit)
	}
	return nil
}

func decodeError(status int, body []byte) *APIError {
	if pr, ok := problem.Decode(status, body); ok {
		return &APIError{Problem: pr, Status: status, Body: body}
	}
	return &APIError{Status: status, Body: body, Malformed: true}
}

// ListPages — GET /public/v1/pages (paginated). Empty Pageable + pageableEditor
// avoids the broken object-styling; the */* body is read raw by Do.
func (c *Client) ListPages(ctx context.Context, page, size int, sort []string, pageTypeID *int64) (*api.PagePageListResult, *APIError) {
	params := &api.ListPagesParams{}
	if pageTypeID != nil {
		params.PageTypeId = pageTypeID
	}
	return Do[api.PagePageListResult](c, func() (*http.Response, error) {
		return c.api.ListPages(ctx, params, pageableEditor(page, size, sort))
	})
}

// SearchPages — GET /public/v1/pages/search?query=. Raw body. allowedPageTypeID,
// when non-nil, scopes the candidates to a page type (used by --property
// PAGE_OUTGOING name resolution).
func (c *Client) SearchPages(ctx context.Context, query string, page, size int, allowedPageTypeID *int64) ([]byte, *APIError) {
	params := &api.SearchPagesParams{Query: query}
	if allowedPageTypeID != nil {
		params.AllowedPageTypeId = allowedPageTypeID
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.SearchPages(ctx, params, pageableEditor(page, size, nil))
	})
}

// GetPage — GET /public/v1/pages/{id} (?expand=). Raw composite body.
func (c *Client) GetPage(ctx context.Context, id int64, expand []string) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetPage(ctx, id, &api.GetPageParams{}, expandEditor(expand))
	})
}

// CreatePage — POST /public/v1/pages. Minimal flag-driven form (name + pageTypeId,
// optional parent/content). For the full form incl. propertyValues use the
// -f form.json route via CreatePageForm.
func (c *Client) CreatePage(ctx context.Context, name string, pageTypeID int64, parentID *int64, content *string) ([]byte, *APIError) {
	return c.CreatePageForm(ctx, api.PageCreateForm{Name: name, PageTypeId: pageTypeID, ParentId: parentID, Content: content})
}

// CreatePageForm — POST /public/v1/pages with a full PageCreateForm (the
// -f form.json route). propertyValues[] are sent verbatim; the server validates
// dataType↔value. No client-side semantic validation.
func (c *Client) CreatePageForm(ctx context.Context, form api.PageCreateForm) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.CreatePage(ctx, form)
	})
}

// Me — GET /public/v1/users/me (self-discovery of the API-key owner).
func (c *Client) Me(ctx context.Context) ([]byte, *APIError) {
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.GetCurrentUser(ctx)
	})
}
