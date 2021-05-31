package atcclient

import (
	"io/fs"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"

	"github.com/chronos-tachyon/roxy/internal/misc"
)

// Close closes all gRPC channels and blocks until all resources are freed.
func (c *ATCClient) Close() error {
	c.mu.Lock()
	closed := c.closed
	connList := make([]*grpc.ClientConn, 0, len(c.connMap))
	for _, cd := range c.connMap {
		if cd.ready && cd.cc != nil {
			connList = append(connList, cd.cc)
			cd.cc = nil
			cd.err = fs.ErrClosed
		}
	}
	c.closed = true
	c.mu.Unlock()

	if closed {
		return fs.ErrClosed
	}

	close(c.closeCh)

	var errs multierror.Error

	for _, cc := range connList {
		if err := cc.Close(); err != nil {
			errs.Errors = append(errs.Errors, err)
		}
	}

	if err := c.cc.Close(); err != nil {
		errs.Errors = append(errs.Errors, err)
	}

	c.wg.Wait()

	return misc.ErrorOrNil(errs)
}
