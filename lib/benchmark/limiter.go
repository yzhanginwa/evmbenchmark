package benchmark

import "context"

type rateLimiter struct {
	sem chan struct{}
}

func newRateLimiter(maxRequests int) *rateLimiter {
	rl := &rateLimiter{sem: make(chan struct{}, maxRequests)}
	for i := 0; i < maxRequests; i++ {
		rl.sem <- struct{}{}
	}
	return rl
}

// acquire blocks until a slot is available or the context is cancelled.
func (rl *rateLimiter) acquire(ctx context.Context) bool {
	select {
	case <-rl.sem:
		return true
	case <-ctx.Done():
		return false
	}
}

// release returns slots back to the limiter (up to capacity).
func (rl *rateLimiter) release(n int) {
	for i := 0; i < n; i++ {
		select {
		case rl.sem <- struct{}{}:
		default:
		}
	}
}
