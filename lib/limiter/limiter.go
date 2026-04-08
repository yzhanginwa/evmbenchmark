package limiter

type RateLimiter struct {
	sem chan struct{}
}

func NewRateLimiter(maxRequests int) *RateLimiter {
	rl := &RateLimiter{sem: make(chan struct{}, maxRequests)}
	for i := 0; i < maxRequests; i++ {
		rl.sem <- struct{}{}
	}
	return rl
}

// Acquire blocks until a slot is available, then claims it.
func (rl *RateLimiter) Acquire() {
	<-rl.sem
}

// IncreaseLimit returns slots back to the limiter (up to capacity).
func (rl *RateLimiter) IncreaseLimit(n int) {
	for i := 0; i < n; i++ {
		select {
		case rl.sem <- struct{}{}:
		default:
		}
	}
}
