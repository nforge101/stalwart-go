package stalwart

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	// domainGetMethod is the JMAP method that reads domains. Verified on
	// Stalwart v1.0.1.
	domainGetMethod = "x:Domain/get"

	// maxDomainIDLen bounds the length of a domain ID.
	maxDomainIDLen = 256
)

// Domain is a mail domain on the Stalwart server.
//
// Only fields verified against a real server are included. More fields are
// added as they are validated.
type Domain struct {
	// ID is the server's identifier for the domain. Use it with Get.
	ID string

	// Name is the domain name, for example "example.com".
	Name string
}

// DomainService performs domain operations. Obtain it with Client.Domains.
//
// DomainService is safe for concurrent use, because the Client it wraps is.
type DomainService struct {
	client *Client
}

// Domains returns the domain operations for the client.
func (c *Client) Domains() DomainService {
	return DomainService{client: c}
}

// domainWire is the JSON form of a domain in an x:Domain/get response.
type domainWire struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// domainGetResult holds the arguments of an x:Domain/get response.
type domainGetResult struct {
	List     []domainWire `json:"list"`
	NotFound []string     `json:"notFound"`
}

// List returns every domain on the server. Stalwart returns all domains in one
// response, so List does not paginate.
//
// List requires the sysDomainGet permission. Without it, the error is a
// *MethodError whose Type is "forbidden", which matches ErrForbidden.
func (s DomainService) List(ctx context.Context) ([]Domain, error) {
	raw, err := s.client.callJMAP(ctx, domainGetMethod, map[string]any{
		"ids":        nil,
		"properties": []string{"name"},
	})
	if err != nil {
		return nil, err
	}

	var res domainGetResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%w: domain list has an unexpected shape", ErrDecode)
	}

	domains := make([]Domain, 0, len(res.List))
	for _, w := range res.List {
		d, err := toDomain(w)
		if err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, nil
}

// Get returns the domain with the given ID.
//
// If the server does not have a domain with that ID, Get returns ErrNotFound.
// An ID must be 1 to 256 printable ASCII characters with no spaces; Get
// reports an invalid ID without contacting the server.
func (s DomainService) Get(ctx context.Context, id string) (Domain, error) {
	if err := validateDomainID(id); err != nil {
		return Domain{}, err
	}

	raw, err := s.client.callJMAP(ctx, domainGetMethod, map[string]any{
		"ids":        []string{id},
		"properties": []string{"name"},
	})
	if err != nil {
		return Domain{}, err
	}

	var res domainGetResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return Domain{}, fmt.Errorf("%w: domain has an unexpected shape", ErrDecode)
	}
	if len(res.List) == 0 {
		return Domain{}, ErrNotFound
	}
	if len(res.List) != 1 {
		return Domain{}, fmt.Errorf("%w: expected one domain, got %d", ErrDecode, len(res.List))
	}

	d, err := toDomain(res.List[0])
	if err != nil {
		return Domain{}, err
	}
	if d.ID != id {
		return Domain{}, fmt.Errorf("%w: returned domain does not match the requested ID", ErrDecode)
	}
	return d, nil
}

// toDomain converts a wire domain, requiring the ID and name that every domain
// has.
func toDomain(w domainWire) (Domain, error) {
	if w.ID == "" || w.Name == "" {
		return Domain{}, fmt.Errorf("%w: domain lacks an ID or name", ErrDecode)
	}
	return Domain{ID: w.ID, Name: w.Name}, nil
}

// validateDomainID reports whether id is an acceptable domain ID. The ID is
// sent in the request body, not in the URL path, so this check guards against
// malformed input rather than path injection.
func validateDomainID(id string) error {
	if id == "" || len(id) > maxDomainIDLen || !isPrintableASCII(id) {
		return fmt.Errorf("stalwart: domain ID must be 1 to %d printable ASCII characters without spaces", maxDomainIDLen)
	}
	return nil
}
