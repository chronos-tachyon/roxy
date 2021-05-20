package membership

import (
	"fmt"
	"net"
)

// type ConflictingAddressesError {{{

// ConflictingAddressesError represents a conflicting attempt to use two
// different IP addresses to refer to the same server.  The ServerSet format
// supports this, but Roxy format does not.
type ConflictingAddressesError struct {
	A net.IP
	B net.IP
}

// Error fulfills the error interface.
func (err ConflictingAddressesError) Error() string {
	return fmt.Sprintf("conflicting IP addresses: %s vs %s", err.A, err.B)
}

var _ error = ConflictingAddressesError{}

// }}}

// type ConflictingZonesError {{{

// ConflictingZonesError represents a conflicting attempt to use two different
// IPv6 zones to refer to the same server.  The ServerSet format supports this,
// but Roxy format does not.
type ConflictingZonesError struct {
	A string
	B string
}

// Error fulfills the error interface.
func (err ConflictingZonesError) Error() string {
	return fmt.Sprintf("conflicting IPv6 zones: %q vs %q", err.A, err.B)
}

var _ error = ConflictingZonesError{}

// }}}
