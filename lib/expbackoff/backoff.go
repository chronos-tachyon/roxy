package expbackoff

import (
	"math"
	"math/rand"
	"time"

	"google.golang.org/grpc/backoff"

	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

type ExpBackoff struct {
	Config backoff.Config
	Rand   *rand.Rand
}

func (impl ExpBackoff) Backoff(retries int) time.Duration {
	if retries < 1 {
		return impl.Config.BaseDelay
	}
	backoff, max := float64(impl.Config.BaseDelay), float64(impl.Config.MaxDelay)
	backoff *= math.Pow(impl.Config.Multiplier, float64(retries))
	if backoff > max {
		backoff = max
	}
	backoff *= 1.0 + impl.Config.Jitter*(impl.Rand.Float64()*2.0-1.0)
	if backoff < 0.0 {
		backoff = 0.0
	}
	return time.Duration(backoff)
}

func BuildDefault() ExpBackoff {
	return ExpBackoff{
		Config: backoff.DefaultConfig,
		Rand:   syncrand.Global(),
	}
}

func Build(cfg backoff.Config) ExpBackoff {
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = backoff.DefaultConfig.BaseDelay
	}
	if cfg.Multiplier <= 0.0 {
		cfg.Multiplier = backoff.DefaultConfig.Multiplier
	}
	if cfg.Jitter <= 0.0 {
		cfg.Jitter = backoff.DefaultConfig.Jitter
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = backoff.DefaultConfig.MaxDelay
	}
	return ExpBackoff{
		Config: cfg,
		Rand:   syncrand.Global(),
	}
}
