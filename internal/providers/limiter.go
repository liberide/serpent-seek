package providers

import (
	"context"
	"strconv"
	"sync"

	"golang.org/x/time/rate"
)

// Per-driver token buckets (golang.org/x/time/rate, BSD-3-Clause — passes the
// license gate). Node-level retry delays cannot protect an upstream rate limit
// when two nodes of the same provider (or parallel user requests) run at once,
// so the limiter lives next to the driver, not the chain.
var (
	limitersMu sync.Mutex
	limiters   = map[string]*rate.Limiter{}
)

// WaitLimit blocks until the per-driver bucket for (code, rps) allows one call.
// rps <= 0 disables throttling. The bucket key includes the rate so instances
// with different overrides never rewrite a shared limiter.
func WaitLimit(ctx context.Context, code string, rps float64) error {
	if rps <= 0 {
		return nil
	}
	key := code + "@" + strconv.FormatFloat(rps, 'g', -1, 64)
	limitersMu.Lock()
	lim, ok := limiters[key]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rps), 1)
		limiters[key] = lim
	}
	limitersMu.Unlock()
	return lim.Wait(ctx)
}

// maxRPSParam resolves the effective rate limit: `max_rps` param override wins,
// then the driver default.
func maxRPSParam(params Params, def float64) float64 {
	if v := params["max_rps"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
