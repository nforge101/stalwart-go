package stalwart

import "errors"

// ErrInvalidConfig is wrapped by errors returned when client configuration is
// invalid, such as a malformed base URL or an out-of-range option. Test for it
// with errors.Is.
//
// Error messages never include user information from the base URL or other
// configured credentials.
var ErrInvalidConfig = errors.New("stalwart: invalid configuration")
