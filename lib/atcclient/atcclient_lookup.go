package atcclient

import (
	"context"
	"io/fs"

	"github.com/rs/zerolog/log"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// Lookup queries any ATC tower for information about the given service or
// shard.  Unlike with most other uses of Key, setting HasShardID to false is a
// wildcard lookup that returns information about all shards (if the service is
// sharded), rather than an assertion that the service is not sharded.
func (c *ATCClient) Lookup(
	ctx context.Context,
	key Key,
) (
	*roxy_v0.LookupResponse,
	error,
) {
	roxyutil.AssertNotNil(&ctx)

	c.wg.Add(1)
	defer c.wg.Done()

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return nil, fs.ErrClosed
	}

	req := &roxy_v0.LookupRequest{
		ServiceName: key.ServiceName,
		ShardId:     key.ShardID,
		HasShardId:  key.HasShardID,
	}

	log.Logger.Trace().
		Str("type", "ATCClient").
		Str("method", "Lookup").
		Stringer("key", key).
		Msg("Call")

	resp, err := c.atc.Lookup(ctx, req)

	if err != nil {
		log.Logger.Trace().
			Str("type", "ATCClient").
			Str("method", "Lookup").
			Stringer("key", key).
			Err(err).
			Msg("Error")
	}

	return resp, err
}

// LookupClients queries any ATC tower for information about the active clients
// of a given shard.
func (c *ATCClient) LookupClients(
	ctx context.Context,
	key Key,
	uniqueID string,
) (
	*roxy_v0.LookupClientsResponse,
	error,
) {
	roxyutil.AssertNotNil(&ctx)

	c.wg.Add(1)
	defer c.wg.Done()

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return nil, fs.ErrClosed
	}

	req := &roxy_v0.LookupClientsRequest{
		ServiceName: key.ServiceName,
		ShardId:     key.ShardID,
		HasShardId:  key.HasShardID,
		Unique:      uniqueID,
	}

	log.Logger.Trace().
		Str("type", "ATCClient").
		Str("method", "LookupClients").
		Stringer("key", key).
		Str("uniqueID", uniqueID).
		Msg("Call")

	resp, err := c.atc.LookupClients(ctx, req)

	if err != nil {
		log.Logger.Trace().
			Str("type", "ATCClient").
			Str("method", "Lookup").
			Stringer("key", key).
			Str("uniqueID", uniqueID).
			Err(err).
			Msg("Error")
	}

	return resp, err
}

// LookupServers queries any ATC tower for information about the active servers
// of a given shard.
func (c *ATCClient) LookupServers(
	ctx context.Context,
	key Key,
	uniqueID string,
) (
	*roxy_v0.LookupServersResponse,
	error,
) {
	roxyutil.AssertNotNil(&ctx)

	c.wg.Add(1)
	defer c.wg.Done()

	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return nil, fs.ErrClosed
	}

	req := &roxy_v0.LookupServersRequest{
		ServiceName: key.ServiceName,
		ShardId:     key.ShardID,
		HasShardId:  key.HasShardID,
		Unique:      uniqueID,
	}

	log.Logger.Trace().
		Str("type", "ATCClient").
		Str("method", "LookupServers").
		Stringer("key", key).
		Str("uniqueID", uniqueID).
		Msg("Call")

	resp, err := c.atc.LookupServers(ctx, req)

	if err != nil {
		log.Logger.Trace().
			Str("type", "ATCClient").
			Str("method", "Lookup").
			Stringer("key", key).
			Str("uniqueID", uniqueID).
			Err(err).
			Msg("Error")
	}

	return resp, err
}
