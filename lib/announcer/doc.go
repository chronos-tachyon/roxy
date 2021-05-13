// Package announcer encapsulates the idea of a server that dynamically
// advertises its presence to clients through various media.
//
// Typical usage:
//
//	// On startup:
//	var a announcer.Announcer
//	impl, err := announcer.NewFoo(...)
//	// check err
//	a.Add(impl)
//
//	// When ready to serve:
//	err = a.Announce(ctx, &membership.Roxy{
//		Ready:       true,
//		IP:          ipAddr,
//		ServerName:  "myserver.example.com", // name on your TLS certificate
//		PrimaryPort: mainPort,
//		AdditionalPorts: map[string]uint16{
//			"foo": fooPort,
//			"bar": barPort,
//		},
//	})
//	// check err
//
//	// When about to stop serving:
//	err = a.Withdraw(ctx)
//	// check err
//
//	// On exit:
//	err = a.Close()
//	// check err
//
package announcer
