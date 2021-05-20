package roxyutil

import (
	"context"
	"fmt"
	"net"
)

// LookupPort is a wrapper around "net".(*Resolver).LookupPort.
func LookupPort(ctx context.Context, res *net.Resolver, network string, service string) (uint16, error) {
	if res == nil {
		res = &net.Resolver{}
	}
	port, err := res.LookupPort(ctx, network, service)
	if err != nil {
		return 0, LookupPortError{
			Net:  network,
			Port: service,
			Err:  err,
		}
	}
	if port < 0 || port >= 65536 {
		return 0, LookupPortError{
			Net:  network,
			Port: service,
			Err: CheckError{
				Message: fmt.Sprintf("port number %d is out of range 0..65535", port),
			},
		}
	}
	return uint16(port), nil
}

// LookupHost is a wrapper around "net".(*Resolver).LookupHost.
func LookupHost(ctx context.Context, res *net.Resolver, hostname string) ([]net.IP, error) {
	if res == nil {
		res = &net.Resolver{}
	}

	records, err := res.LookupHost(ctx, hostname)
	if err != nil {
		return nil, LookupHostError{
			Host: hostname,
			Err:  err,
		}
	}

	addrs := make([]net.IP, len(records))
	for index, record := range records {
		ip := net.ParseIP(record)
		if ip == nil {
			return nil, LookupHostError{
				Host: hostname,
				Err: IPError{
					IP:  record,
					Err: ErrFailedToMatch,
				},
			}
		}
		addrs[index] = ip
	}

	return addrs, nil
}

// LookupSRV is a wrapper around "net".(*Resolver).LookupSRV.
func LookupSRV(ctx context.Context, res *net.Resolver, service, network, domain string) (string, []*net.SRV, error) {
	if res == nil {
		res = &net.Resolver{}
	}

	cname, records, err := res.LookupSRV(ctx, service, network, domain)
	if err != nil {
		return "", nil, LookupSRVError{
			Service: service,
			Network: network,
			Domain:  domain,
			Err:     err,
		}
	}

	return cname, records, nil
}
