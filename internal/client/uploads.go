package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	"github.com/42BV/normatik-cli/internal/localfile"
)

// UploadAttachment uploads a file as a page attachment (multipart/form-data,
// part name "file") — POST /public/v1/pages/{pageId}/file-attachments.
func (c *Client) UploadAttachment(ctx context.Context, pageID int64, filePath string) ([]byte, *APIError) {
	body, contentType, err := multipartFile(filePath)
	if err != nil {
		return nil, &APIError{Transport: err}
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.UploadFileAttachmentWithBody(ctx, pageID, contentType, body)
	})
}

// UploadPageImage uploads a file as a page image — POST /public/v1/pages/{pageId}/images.
func (c *Client) UploadPageImage(ctx context.Context, pageID int64, filePath string) ([]byte, *APIError) {
	body, contentType, err := multipartFile(filePath)
	if err != nil {
		return nil, &APIError{Transport: err}
	}
	return c.DoRaw(func() (*http.Response, error) {
		return c.api.UploadImageWithBody(ctx, pageID, contentType, body)
	})
}

// maxUploadBytes is a client-side sanity cap on upload size (aligned with the
// server's 50 MB attachment limit) so a special or oversized file cannot force
// the CLI to buffer unbounded data (NORMATIK-21, CWE-400).
const maxUploadBytes int64 = 50 << 20

// multipartFile builds a multipart/form-data body with a single "file" part
// (the field the upload endpoints expect) and returns it plus the content-type
// header (with the generated boundary).
//
// NORMATIK-21 (CWE-367/CWE-400/CWE-59): the file is opened no-follow and validated as a
// regular file on the OPENED descriptor (localfile.Open closes the check/open TOCTOU
// window), size-capped BEFORE any bytes are read, and streamed straight into the multipart
// part (no second full-file copy in memory).
func multipartFile(path string) (io.Reader, string, error) {
	f, info, err := localfile.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()
	if info.Size() > maxUploadBytes {
		return nil, "", fmt.Errorf("%s is %d bytes, which exceeds the %d-byte upload limit", path, info.Size(), maxUploadBytes)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	// Set the part's Content-Type from the extension so image uploads pass the
	// server's image-type validation (default would be application/octet-stream).
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(filepath.Base(path))
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escaped))
	h.Set("Content-Type", ct)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	// Stream the file straight into the part (no full-file []byte first). The
	// LimitReader is a belt-and-braces guard against growth between fstat and read.
	if _, err := io.Copy(part, io.LimitReader(f, maxUploadBytes)); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}
