package stalwart

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	// maxErrorRead limits how much of a non-2xx response body is read.
	maxErrorRead = 4 << 10

	// maxErrorSnippet limits how much sanitized error text is kept in
	// APIError.Message.
	maxErrorSnippet = 512

	// maxResponseBody limits how much of a successful response body is decoded.
	maxResponseBody = 16 << 20
)

// errInvalidPath is returned for request paths that are not safe to send.
var errInvalidPath = errors.New("stalwart: invalid request path")

// endpoint returns the absolute URL for p, a path relative to the base URL.
//
// p must start with "/" and contain only printable ASCII characters. It must
// not contain '?', '#', or '\', and it must not contain "." or ".." segments,
// including percent-encoded forms. Dynamic values, such as resource names,
// must be escaped with url.PathEscape by the caller, so that a value cannot
// add path segments.
func (c *Client) endpoint(p string) (*url.URL, error) {
	if !strings.HasPrefix(p, "/") || !isPrintableASCII(p) || strings.ContainsAny(p, "?#\\") {
		return nil, errInvalidPath
	}
	for _, seg := range strings.Split(p, "/") {
		decoded, err := url.PathUnescape(seg)
		if err != nil || decoded == "." || decoded == ".." {
			return nil, errInvalidPath
		}
	}

	// Build from the escaped base URL and let url.Parse handle the escaping of
	// p, so encoded slashes in a value like "a%2Fb" stay one segment.
	u, err := url.Parse(strings.TrimSuffix(c.baseURL.String(), "/") + p)
	if err != nil {
		return nil, errInvalidPath
	}
	return u, nil
}

// newRequest builds an authenticated request. If body is non-nil, it is encoded
// as JSON.
func (c *Client) newRequest(ctx context.Context, method, p string, body any) (*http.Request, error) {
	u, err := c.endpoint(p)
	if err != nil {
		return nil, err
	}

	var payload io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("stalwart: encode request body: %w", err)
		}
		payload = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), payload)
	if err != nil {
		return nil, fmt.Errorf("stalwart: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.auth.authorize(req)
	return req, nil
}

// doJSON sends a request and handles the response.
//
// If in is non-nil, it is encoded as the JSON request body. For a 2xx response,
// the body is decoded into out, unless out is nil, in which case the body is
// discarded. A non-2xx response returns an *APIError. Transport failures wrap
// ErrTransport, and decoding failures wrap ErrDecode. The response body is
// always closed.
func (c *Client) doJSON(ctx context.Context, method, p string, in, out any) error {
	req, err := c.newRequest(ctx, method, p, in)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransport, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// A read error here only truncates the diagnostic excerpt, so it is ignored.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorRead))
		return &APIError{StatusCode: resp.StatusCode, Message: c.errorSnippet(raw)}
	}

	limited := io.LimitReader(resp.Body, maxResponseBody)
	if out == nil {
		_, _ = io.Copy(io.Discard, limited)
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("%w: %w", ErrDecode, err)
	}
	return nil
}

// errorSnippet turns a raw error body into a short, safe excerpt.
//
// The credential is redacted before truncation. Because the token is at most
// maxBearerTokenLen bytes and the read limit is far larger, a whole token is
// always visible to redaction. Control characters are replaced, and invalid
// UTF-8 is removed.
func (c *Client) errorSnippet(raw []byte) string {
	text := c.auth.redact(string(raw))
	text = strings.ToValidUTF8(text, "\uFFFD")
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	text = strings.TrimSpace(text)
	if len(text) > maxErrorSnippet {
		text = strings.ToValidUTF8(text[:maxErrorSnippet], "")
	}
	return text
}
