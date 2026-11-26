package stalwart

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// jmapPath is the JMAP API endpoint, relative to the base URL. Verified on
	// Stalwart v1.0.1.
	jmapPath = "/jmap/"

	// jmapCallID is the call ID of the single method call in each request.
	jmapCallID = "c1"

	// jmapCoreCapability and stalwartCapability are the capabilities that each
	// request declares. Both are verified on Stalwart v1.0.1.
	jmapCoreCapability = "urn:ietf:params:jmap:core"
	stalwartCapability = "urn:stalwart:jmap"

	// jmapErrorMethod is the method name of a JMAP error response.
	jmapErrorMethod = "error"
)

// jmapRequest is the body of a JMAP API request. Each method call is the
// three-element array [name, arguments, callId].
type jmapRequest struct {
	Using       []string `json:"using"`
	MethodCalls [][]any  `json:"methodCalls"`
}

// jmapResponse is the body of a JMAP API response. Each method response is the
// three-element array [name, arguments, callId].
type jmapResponse struct {
	MethodResponses [][]json.RawMessage `json:"methodResponses"`
}

// jmapErrorArgs holds the arguments of an "error" method response.
type jmapErrorArgs struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// callJMAP sends one JMAP method call to the management endpoint and returns
// the arguments of the matching method response.
//
// If Stalwart rejects the method inside an HTTP 200 response, callJMAP returns
// a *MethodError. HTTP-level failures return an *APIError, as with doJSON. A
// response that does not have the expected shape wraps ErrDecode.
//
// Error text never includes server-supplied names or identifiers. Only the
// error type and the sanitized description are reported.
func (c *Client) callJMAP(ctx context.Context, method string, args any) (json.RawMessage, error) {
	req := jmapRequest{
		Using:       []string{jmapCoreCapability, stalwartCapability},
		MethodCalls: [][]any{{method, args, jmapCallID}},
	}

	var resp jmapResponse
	if err := c.doJSON(ctx, http.MethodPost, jmapPath, req, &resp); err != nil {
		return nil, err
	}

	if len(resp.MethodResponses) != 1 {
		return nil, fmt.Errorf("%w: expected one method response, got %d", ErrDecode, len(resp.MethodResponses))
	}
	mr := resp.MethodResponses[0]
	if len(mr) != 3 {
		return nil, fmt.Errorf("%w: method response has %d elements, want 3", ErrDecode, len(mr))
	}

	var name, callID string
	if err := json.Unmarshal(mr[0], &name); err != nil {
		return nil, fmt.Errorf("%w: method response name is not a string", ErrDecode)
	}
	if err := json.Unmarshal(mr[2], &callID); err != nil || callID != jmapCallID {
		return nil, fmt.Errorf("%w: method response has an unexpected call ID", ErrDecode)
	}

	if name == jmapErrorMethod {
		return nil, c.methodError(mr[1])
	}
	if name != method {
		return nil, fmt.Errorf("%w: method response name does not match the request", ErrDecode)
	}
	return mr[1], nil
}

// methodError converts the arguments of an "error" method response into a
// *MethodError. The type is required; the description may be absent.
func (c *Client) methodError(raw json.RawMessage) error {
	var args jmapErrorArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Type == "" {
		return fmt.Errorf("%w: error method response has no valid type", ErrDecode)
	}
	return &MethodError{
		Type:        c.errorSnippet([]byte(args.Type)),
		Description: c.errorSnippet([]byte(args.Description)),
	}
}
