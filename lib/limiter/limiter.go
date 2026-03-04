package limiter

import (
	"sync"
)

type RateLimiter struct {
	mu              sync.Mutex
	cond            *sync.Cond
	remaining       int
	max             int
	blockedAcquires int64
}

func NewRateLimiter(maxRequests int) *RateLimiter {
	rl := &RateLimiter{remaining: maxRequests, max: maxRequests}
	rl.cond = sync.NewCond(&rl.mu)
	return rl
}

// Acquire blocks until a slot is available, then claims it.
func (rl *RateLimiter) Acquire() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.remaining == 0 {
		rl.blockedAcquires++
	}
	for rl.remaining == 0 {
		rl.cond.Wait()
	}
	rl.remaining--
}

func (rl *RateLimiter) IncreaseLimit(newCapacity int) {
	rl.mu.Lock()
	rl.remaining += newCapacity
	if rl.remaining > rl.max {
		rl.remaining = rl.max
	}
	rl.cond.Broadcast()
	rl.mu.Unlock()
}

// SetMax adjusts the maximum mempool size. If increasing, adds the difference
// to remaining. If decreasing, caps remaining at the new max.
func (rl *RateLimiter) SetMax(newMax int) {
	rl.mu.Lock()
	if newMax > rl.max {
		rl.remaining += newMax - rl.max
	} else {
		if rl.remaining > newMax {
			rl.remaining = newMax
		}
	}
	rl.max = newMax
	rl.cond.Broadcast()
	rl.mu.Unlock()
}

func (rl *RateLimiter) GetMax() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.max
}

func (rl *RateLimiter) BlockedAcquires() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.blockedAcquires
}
