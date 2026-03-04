package limiter

import (
	"sync"
)

type RateLimiter struct {
	mu        sync.Mutex
	cond      *sync.Cond
	remaining int
}

func NewRateLimiter(maxRequests int) *RateLimiter {
	rl := &RateLimiter{remaining: maxRequests}
	rl.cond = sync.NewCond(&rl.mu)
	return rl
}

// Acquire blocks until a slot is available, then claims it.
func (rl *RateLimiter) Acquire() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for rl.remaining == 0 {
		rl.cond.Wait()
	}
	rl.remaining--
}

func (rl *RateLimiter) IncreaseLimit(newCapacity int) {
	rl.mu.Lock()
	rl.remaining += newCapacity
	rl.cond.Broadcast()
	rl.mu.Unlock()
}
