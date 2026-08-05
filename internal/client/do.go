package client

import (
	"encoding/json"
	"io"
	"net/http"
)

const (
	jsonResponseLimit  int64 = 32 << 20
	errorResponseLimit int64 = 1 << 20
	downloadLimit      int64 = 64 << 20
)

// LowLevel is a thunk that performs exactly one generated low-level oapi call.
// The wrapper method closes over ctx, path params, body and request-editors, so
// Do/DoRaw stay completely generic and never see resource-specific shapes.
type LowLevel func() (*http.Response, error)

// exec runs the call, reads the body and classifies the status: 2xx → (body,nil),
// any non-2xx (incl. 3xx) → decodeError. Conditional GETs that must treat 304 as
// a valid answer use DoConditional, not exec/DoRaw.
func (c *Client) exec(call LowLevel) (body []byte, status int, apiErr *APIError) {
	resp, err := call()
	if err != nil {
		return nil, 0, fail(err)
	}
	defer func() { _ = resp.Body.Close() }()
	limit := errorResponseLimit
	if resp.StatusCode/100 == 2 {
		limit = jsonResponseLimit
	}
	b, rerr := readBounded(resp.Body, limit, resp.StatusCode)
	if rerr != nil {
		if apiErr := APIErrorFromRead(rerr); apiErr != nil {
			return nil, resp.StatusCode, apiErr
		}
		return nil, resp.StatusCode, fail(rerr)
	}
	if resp.StatusCode/100 == 2 {
		return b, resp.StatusCode, nil
	}
	return b, resp.StatusCode, decodeError(resp.StatusCode, b)
}

// DoRaw executes a low-level call and returns the raw 2xx body. It centralises
// the two API-wide invariants: non-2xx → ProblemDetail/APIError via decodeError,
// and the `*/*` content-type quirk → the body is always read raw here (the
// generated client gives no typed JSON200 for `*/*`). NOTE: 3xx (incl. 304) is
// treated as an error; ETag/If-None-Match GETs must use DoConditional.
func (c *Client) DoRaw(call LowLevel) ([]byte, *APIError) {
	body, _, apiErr := c.exec(call)
	return body, apiErr
}

// DoConditional is for ETag/If-None-Match GETs: a 304 is returned as
// notModified=true (NOT an error), 2xx as the body, other non-2xx as APIError.
func (c *Client) DoConditional(call LowLevel) (body []byte, notModified bool, apiErr *APIError) {
	resp, err := call()
	if err != nil {
		return nil, false, fail(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		return nil, true, nil
	}
	limit := errorResponseLimit
	if resp.StatusCode/100 == 2 {
		limit = jsonResponseLimit
	}
	b, rerr := readBounded(resp.Body, limit, resp.StatusCode)
	if rerr != nil {
		if apiErr := APIErrorFromRead(rerr); apiErr != nil {
			return nil, false, apiErr
		}
		return nil, false, fail(rerr)
	}
	if resp.StatusCode/100 != 2 {
		return b, false, decodeError(resp.StatusCode, b)
	}
	return b, false, nil
}

// DoDownload is the streaming counterpart of DoConditional. On a successful
// 2xx response the caller owns and must close body. Every other path closes the
// response body before returning.
func (c *Client) DoDownload(call LowLevel) (body io.ReadCloser, notModified bool, apiErr *APIError) {
	resp, err := call()
	if err != nil {
		return nil, false, fail(err)
	}
	if resp.StatusCode == http.StatusNotModified {
		_ = resp.Body.Close()
		return nil, true, nil
	}
	if resp.StatusCode/100 != 2 {
		defer func() { _ = resp.Body.Close() }()
		body, readErr := readBounded(resp.Body, errorResponseLimit, resp.StatusCode)
		if readErr != nil {
			if limitErr := APIErrorFromRead(readErr); limitErr != nil {
				return nil, false, limitErr
			}
			return nil, false, fail(readErr)
		}
		return nil, false, decodeError(resp.StatusCode, body)
	}
	if resp.ContentLength > downloadLimit {
		_ = resp.Body.Close()
		return nil, false, responseTooLarge(resp.StatusCode, downloadLimit)
	}
	return &boundedReadCloser{
		body:      resp.Body,
		remaining: downloadLimit,
		status:    resp.StatusCode,
		limit:     downloadLimit,
	}, false, nil
}

func readBounded(r io.Reader, limit int64, status int) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, &ResponseTooLargeError{Status: status, Limit: limit}
	}
	return body, nil
}

type boundedReadCloser struct {
	body      io.ReadCloser
	remaining int64
	status    int
	limit     int64
}

func (r *boundedReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.body.Read(extra[:])
		if n > 0 {
			return 0, &ResponseTooLargeError{Status: r.status, Limit: r.limit}
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.body.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func (r *boundedReadCloser) Close() error { return r.body.Close() }

// Do is DoRaw + a typed JSON unmarshal into T. An empty 2xx body (e.g. 204 No
// Content) yields the zero value of T, not an error. A non-empty body that fails
// to unmarshal is a malformed-response APIError carrying the real status code.
func Do[T any](c *Client, call LowLevel) (*T, *APIError) {
	body, status, apiErr := c.exec(call)
	if apiErr != nil {
		return nil, apiErr
	}
	var out T
	if len(body) == 0 {
		return &out, nil
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &APIError{Status: status, Body: body, Malformed: true}
	}
	return &out, nil
}
