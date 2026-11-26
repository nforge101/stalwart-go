package stalwart

import (
	"errors"
	"fmt"
)

// ErrInvalidConfig is wrapped by errors returned when client configuration is
// invalid, such as a malformed base URL, a missing credential, or an
// out-of-range option. Test for it with errors.Is.
//
// Error messages never include user information from the base URL or any
// configured credential.
var ErrInvalidConfig = errors.New("stalwart: invalid configuration")

// ErrUnauthorized reports that Stalwart rejected the request's credentials
// (HTTP 401). It is wrapped by the *APIError returned for that response.
var ErrUnauthorized = errors.New("stalwart: unauthorized")

// ErrForbidden reports that Stalwart accepted the credentials but the account
// lacks permission for the operation. It is wrapped by the *APIError returned
// for an HTTP 403 response, and by a *MethodError whose Type is "forbidden".
var ErrForbidden = errors.New("stalwart: forbidden")

// ErrTransport reports that the request could not be sent or no response was
// received. This includes connection failures, timeouts, and cancellation. The
// underlying error is wrapped, so errors.Is and errors.As can inspect it, for
// example for context.Canceled or a net.Error whose Timeout method returns true.
var ErrTransport = errors.New("stalwart: transport failure")

// ErrDecode reports that a successful response body was not valid JSON for the
// expected value. The response body itself is not included in the error.
var ErrDecode = errors.New("stalwart: decode response")

// APIError reports a non-2xx HTTP response from Stalwart.
//
// StatusCode is the HTTP status code. Message is a sanitized, truncated excerpt
// of the response body: control characters are replaced, the client's
// credential is redacted, and it is limited to a fixed size. Message may be
// empty.
//
// For 401 and 403 responses, errors.Is matches ErrUnauthorized and ErrForbidden
// respectively. Other statuses, including 404, do not match a sentinel: a 404
// alone does not show that an operation is unsupported.
type APIError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("stalwart: HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("stalwart: HTTP %d: %s", e.StatusCode, e.Message)
}

// Unwrap returns ErrUnauthorized for 401 responses and ErrForbidden for 403
// responses. For other statuses it returns nil.
func (e *APIError) Unwrap() error {
	switch e.StatusCode {
	case 401:
		return ErrUnauthorized
	case 403:
		return ErrForbidden
	default:
		return nil
	}
}

// MethodError reports that Stalwart rejected a JMAP method call. The HTTP
// request succeeded: the rejection arrives as an "error" method response inside
// an HTTP 200 body, so no *APIError is involved. Callers must check for
// MethodError to detect these rejections.
//
// Type is the JMAP error type, such as "forbidden". Description is a sanitized,
// truncated excerpt of the server's description, with the same rules as
// APIError.Message. It may be empty.
//
// errors.Is matches ErrForbidden when Type is "forbidden". Other types match no
// sentinel.
type MethodError struct {
	Type        string
	Description string
}

// Error implements the error interface.
func (e *MethodError) Error() string {
	if e.Description == "" {
		return fmt.Sprintf("stalwart: method error %s", e.Type)
	}
	return fmt.Sprintf("stalwart: method error %s: %s", e.Type, e.Description)
}

// Unwrap returns ErrForbidden when Type is "forbidden" and nil otherwise.
func (e *MethodError) Unwrap() error {
	if e.Type == "forbidden" {
		return ErrForbidden
	}
	return nil
}
