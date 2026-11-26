package stalwart

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	// domainSetMethod is the JMAP method that creates, updates, and destroys
	// domains. Verified on Stalwart v1.0.1.
	domainSetMethod = "x:Domain/set"

	// dkimSignatureObject is the only linked-object type that Delete removes.
	// Stalwart creates one for each domain that has automatic DKIM management.
	dkimSignatureObject    = "DkimSignature"
	dkimSignatureSetMethod = "x:DkimSignature/set"

	// createRef is the creation ID of the one object in a create request.
	createRef = "new"

	// maxDomainNameLen is the longest domain name the SDK sends, in bytes. DNS
	// allows 253 characters in a name.
	maxDomainNameLen = 253

	// maxLabelLen is the longest label in a domain name, in bytes.
	maxLabelLen = 63
)

// DomainPatch changes some properties of a domain. A nil field is left
// unchanged. Only properties verified against Stalwart v1.0.1 are included.
type DomainPatch struct {
	// Description is the domain's description. Set it to an empty string to
	// clear it.
	Description *string

	// Enable turns the domain on or off.
	Enable *bool
}

// LinkedObject identifies an object that blocks deleting a domain.
type LinkedObject struct {
	// Type is the object's type, for example "DkimSignature".
	Type string

	// ID is the server's identifier for the object.
	ID string
}

// LinkedObjectsError reports that Delete could not delete a domain because
// objects linked to it remain. Delete removes linked DkimSignature objects
// itself. If the domain has any other linked type, Delete removes nothing and
// returns this error. Objects lists the linked objects that remain. The error
// text does not include their IDs.
type LinkedObjectsError struct {
	Objects []LinkedObject
}

// Error implements the error interface.
func (e *LinkedObjectsError) Error() string {
	return fmt.Sprintf("stalwart: domain has %d linked object(s) that Delete does not remove", len(e.Objects))
}

// setResponse holds the arguments of a JMAP /set response (RFC 8620,
// section 5.3). Only the fields the SDK reads are declared.
type setResponse struct {
	Created      map[string]json.RawMessage `json:"created"`
	NotCreated   map[string]json.RawMessage `json:"notCreated"`
	Updated      map[string]json.RawMessage `json:"updated"`
	NotUpdated   map[string]json.RawMessage `json:"notUpdated"`
	Destroyed    []string                   `json:"destroyed"`
	NotDestroyed map[string]json.RawMessage `json:"notDestroyed"`
}

// Create adds a domain with the given name and returns it. Create checks the
// name before it sends anything; see validateDomainName. Stalwart applies its
// own defaults, including an automatic DKIM signature, so Delete will later
// remove that signature.
//
// The returned Domain's Name is the name as sent. Create returns ErrAlreadyExists
// if a domain with that name already exists.
func (s DomainService) Create(ctx context.Context, name string) (Domain, error) {
	if err := validateDomainName(name); err != nil {
		return Domain{}, err
	}
	res, err := callSet(ctx, s.client, domainSetMethod, map[string]any{
		"create": map[string]any{createRef: map[string]any{"name": name}},
	})
	if err != nil {
		return Domain{}, err
	}
	if obj, ok := res.Created[createRef]; ok {
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(obj, &created); err != nil || created.ID == "" || !isPrintableASCII(created.ID) {
			return Domain{}, fmt.Errorf("%w: created domain has no usable ID", ErrDecode)
		}
		return Domain{ID: created.ID, Name: name}, nil
	}
	if rej, ok := res.NotCreated[createRef]; ok {
		return Domain{}, rejectionError(s.client, rej)
	}
	return Domain{}, fmt.Errorf("%w: create is in neither created nor notCreated", ErrDecode)
}

// Update changes the properties that patch sets on the domain with the given
// ID. It returns ErrNotFound if no domain has that ID. A patch that sets no
// properties is rejected without contacting the server.
func (s DomainService) Update(ctx context.Context, id string, patch DomainPatch) error {
	if err := validateDomainID(id); err != nil {
		return err
	}
	props := map[string]any{}
	if patch.Description != nil {
		props["description"] = *patch.Description
	}
	if patch.Enable != nil {
		props["enable"] = *patch.Enable
	}
	if len(props) == 0 {
		return fmt.Errorf("stalwart: domain patch sets no properties")
	}

	res, err := callSet(ctx, s.client, domainSetMethod, map[string]any{
		"update": map[string]any{id: props},
	})
	if err != nil {
		return err
	}
	if _, ok := res.Updated[id]; ok {
		return nil
	}
	if rej, ok := res.NotUpdated[id]; ok {
		return rejectionError(s.client, rej)
	}
	return fmt.Errorf("%w: update is in neither updated nor notUpdated", ErrDecode)
}

// Delete removes the domain with the given ID. It returns ErrNotFound if no
// domain has that ID.
//
// Stalwart refuses to delete a domain while objects are linked to it. If the
// linked objects are DkimSignature objects, Delete removes them and then deletes
// the domain. Removing a DKIM signature is irreversible: mail that the domain
// signs with that key stops verifying. If any other type is linked, Delete
// removes nothing and returns a *LinkedObjectsError.
//
// Removing a DKIM signature needs permission to destroy DkimSignature objects,
// which is separate from the domain permissions. Without it, Delete returns an
// error that matches ErrForbidden.
func (s DomainService) Delete(ctx context.Context, id string) error {
	if err := validateDomainID(id); err != nil {
		return err
	}
	rej, err := s.destroyDomain(ctx, id)
	if err != nil || rej == nil {
		return err
	}
	if rejectionType(rej) != "objectIsLinked" {
		return rejectionError(s.client, rej)
	}

	// Check every linked object before removing any, so that a domain with an
	// unexpected linked type is left untouched.
	linked := parseLinked(rej)
	for _, l := range linked {
		if l.Type != dkimSignatureObject {
			return &LinkedObjectsError{Objects: linked}
		}
		if l.ID == "" || !isPrintableASCII(l.ID) {
			return fmt.Errorf("%w: a linked object has no usable ID", ErrDecode)
		}
	}
	for _, l := range linked {
		if err := s.destroyDKIMSignature(ctx, l.ID); err != nil {
			return fmt.Errorf("removing a linked %s before deleting the domain: %w", dkimSignatureObject, err)
		}
	}

	rej, err = s.destroyDomain(ctx, id)
	if err != nil || rej == nil {
		return err
	}
	if rejectionType(rej) == "objectIsLinked" {
		return &LinkedObjectsError{Objects: parseLinked(rej)}
	}
	return rejectionError(s.client, rej)
}

// destroyDomain makes one destroy request for the domain. It returns a nil
// rejection when the domain was destroyed, and the server's rejection otherwise.
func (s DomainService) destroyDomain(ctx context.Context, id string) (json.RawMessage, error) {
	res, err := callSet(ctx, s.client, domainSetMethod, map[string]any{
		"destroy": []string{id},
	})
	if err != nil {
		return nil, err
	}
	if slices.Contains(res.Destroyed, id) {
		return nil, nil
	}
	if rej, ok := res.NotDestroyed[id]; ok {
		return rej, nil
	}
	return nil, fmt.Errorf("%w: destroy is in neither destroyed nor notDestroyed", ErrDecode)
}

// destroyDKIMSignature removes one DkimSignature object. A signature that is
// already gone counts as removed.
func (s DomainService) destroyDKIMSignature(ctx context.Context, id string) error {
	res, err := callSet(ctx, s.client, dkimSignatureSetMethod, map[string]any{
		"destroy": []string{id},
	})
	if err != nil {
		return err
	}
	if slices.Contains(res.Destroyed, id) {
		return nil
	}
	if rej, ok := res.NotDestroyed[id]; ok {
		if rejectionType(rej) == "notFound" {
			return nil
		}
		return rejectionError(s.client, rej)
	}
	return fmt.Errorf("%w: destroy is in neither destroyed nor notDestroyed", ErrDecode)
}

// callSet sends one /set method call and decodes its result.
func callSet(ctx context.Context, c *Client, method string, args map[string]any) (setResponse, error) {
	raw, err := c.callJMAP(ctx, method, args)
	if err != nil {
		return setResponse{}, err
	}
	var res setResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return setResponse{}, fmt.Errorf("%w: response is not a set result", ErrDecode)
	}
	return res, nil
}

// rejectionType returns the type field of a rejection, or "" if it has none.
func rejectionType(rej json.RawMessage) string {
	var r struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(rej, &r) != nil {
		return ""
	}
	return r.Type
}

// rejectionError maps a server rejection of one object to an error. notFound
// maps to ErrNotFound and primaryKeyViolation to ErrAlreadyExists. Any other
// type is returned as a *RejectedError with its description sanitized.
func rejectionError(c *Client, rej json.RawMessage) error {
	typ := rejectionType(rej)
	switch typ {
	case "":
		return fmt.Errorf("%w: rejection has no type", ErrDecode)
	case "notFound":
		return ErrNotFound
	case "primaryKeyViolation":
		return fmt.Errorf("%w: a domain with that name exists", ErrAlreadyExists)
	}
	var r struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(rej, &r)
	return &RejectedError{
		Type:        c.errorSnippet([]byte(typ)),
		Description: c.errorSnippet([]byte(r.Description)),
	}
}

// parseLinked returns the objects listed in an objectIsLinked rejection.
func parseLinked(rej json.RawMessage) []LinkedObject {
	var r struct {
		Linked []struct {
			Object string `json:"object"`
			ID     string `json:"id"`
		} `json:"linkedObjects"`
	}
	if json.Unmarshal(rej, &r) != nil {
		return nil
	}
	out := make([]LinkedObject, 0, len(r.Linked))
	for _, l := range r.Linked {
		out = append(out, LinkedObject{Type: l.Object, ID: l.ID})
	}
	return out
}

// validateDomainName reports whether name can be sent as a domain name. The
// rules are applied before any request: at least two dot-separated labels, each
// 1 to 63 letters, digits, or hyphens, with no leading or trailing hyphen, and
// at most 253 bytes in total. Stalwart 1.0.1 accepts names with spaces, so the
// server does not enforce these rules by itself.
func validateDomainName(name string) error {
	bad := fmt.Errorf("stalwart: invalid domain name: use at least two dot-separated labels of letters, digits, or hyphens, up to %d characters in total", maxDomainNameLen)
	if len(name) < 3 || len(name) > maxDomainNameLen {
		return bad
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return bad
	}
	for _, l := range labels {
		if l == "" || len(l) > maxLabelLen || l[0] == '-' || l[len(l)-1] == '-' {
			return bad
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return bad
			}
		}
	}
	return nil
}
