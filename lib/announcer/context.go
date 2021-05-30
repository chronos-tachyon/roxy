package announcer

import (
	"context"
)

type contextKey string

// DeclaredCPSContextKey is the context key for the declared maximum
// cost-per-second which the server is able to deliver.  This is reported by
// the ATC announcer for load-balancing purposes.
const DeclaredCPSContextKey contextKey = "roxy.announcer.DeclaredCPS"

// WithDeclaredCPS attaches the declared maximum cost-per-second to the given
// Context.
func WithDeclaredCPS(ctx context.Context, cps float64) context.Context {
	return context.WithValue(ctx, DeclaredCPSContextKey, cps)
}

// GetDeclaredCPS retrieves the declared maximum cost-per-second, or 0.
func GetDeclaredCPS(ctx context.Context) float64 {
	cps, _ := ctx.Value(DeclaredCPSContextKey).(float64)
	return cps
}
