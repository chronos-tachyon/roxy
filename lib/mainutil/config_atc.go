package mainutil

import (
	"context"

	"github.com/chronos-tachyon/roxy/lib/atcclient"
)

type ATCClientConfig struct {
	GRPCClientConfig
}

type accJSON struct {
	gccJSON
}

func (acc ATCClientConfig) NewClient(ctx context.Context) (*atcclient.ATCClient, error) {
	if !acc.Enabled {
		return nil, nil
	}

	tlsConfig, err := acc.TLS.MakeTLS("")
	if err != nil {
		return nil, err
	}

	cc, err := acc.Dial(ctx)
	if err != nil {
		return nil, err
	}

	return atcclient.New(cc, tlsConfig)
}

func (acc ATCClientConfig) toAlt() *accJSON {
	tmp := acc.GRPCClientConfig.toAlt()
	if tmp == nil {
		return nil
	}
	return &accJSON{*tmp}
}

func (alt *accJSON) toStd() (ATCClientConfig, error) {
	if alt == nil {
		return ATCClientConfig{}, nil
	}
	tmp, err := alt.gccJSON.toStd()
	if err != nil {
		return ATCClientConfig{}, err
	}
	return ATCClientConfig{tmp}, nil
}

func (acc ATCClientConfig) postprocess() (ATCClientConfig, error) {
	tmp, err := acc.GRPCClientConfig.postprocess()
	if err != nil {
		return ATCClientConfig{}, err
	}
	return ATCClientConfig{tmp}, nil
}
