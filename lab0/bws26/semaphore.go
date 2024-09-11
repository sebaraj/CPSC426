package lab0

import (
	"context"
	"sync/atomic"
)

type waiting struct{}

// Semaphore mirrors Go's library package `semaphore.Weighted`
// but with a smaller, simpler interface.
//
// Recall that a counting semaphore has two jobs:
//   - keep track of a number of available resources
//   - when resources are depleted, block waiters and resume them
//     when resources are available
type Semaphore struct {
	signal    chan waiting
	available atomic.Int64

	// You may add any other state here. You are also free to remove
	// or modify any existing members.

	// You may not use semaphore.Weighted in your implementation.
}

func NewSemaphore() *Semaphore {
	return &Semaphore{
		signal: make(chan waiting, 1),
	}
}

// func NewSemaphore(max ...int) *Semaphore {
// 	var count int64
// 	if len(max) != 1 {
// 		count = 1
// 	} else {
// 		count = int64(max[0])
// 	}
// 	s := Semaphore{
// 		signal: make(chan waiting, count),
// 		max:    count,
// 	}
// 	// s.available.Store(0)
// 	if len(max) != 1 {
// 		s.available.Store(0)
// 	} else {
// 		s.available.Store(count)
// 	}
// 	return &s
// }

// Post increments the semaphore value by one. If there are any
// callers waiting, it signals exactly one to wake up.
//
// Analagous to Release(1) in semaphore.Weighted. One important difference
// is that calling Release before any Acquire will panic in semaphore.Weighted,
// but calling Post() before Wait() should neither block nor panic in our interface.
func (s *Semaphore) Post() {
	s.available.Add(1)

	select {
	case s.signal <- waiting{}:
	default:
	}
}

// func (s *Semaphore) Post() {
// 	currentAvailable := s.available.Load()
// 	if currentAvailable >= s.max {
// 		return
// 	}
// 	if s.available.CompareAndSwap(currentAvailable, currentAvailable+1) {
// 		select {
// 		case s.signal <- waiting{}:
// 		default:
// 		}
// 		return
// 	}
//
// 	//select {
// 	//case s.signal <- struct{}{}:
// 	//default:
// 	//}
// 	//s.available.Add(1)
// }

// Wait decrements the semaphore value by one, if there are resources
// remaining from previous calls to Post. If there are no resources remaining,
// waits until resources are available or until the context is done, whichever
// is first.
//
// If the context is done with an error, returns that error. Returns `nil`
// in all other cases.
//
// Analagous to Acquire(ctx, 1) in semaphore.Weighted.
func (s *Semaphore) Wait(ctx context.Context) error {
	for {
		v := s.available.Load()
		if v > 0 {
			if s.available.CompareAndSwap(v, v-1) {
				return nil
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.signal:
		}
	}
}

// func (s *Semaphore) Wait(ctx context.Context) error {
// 	select {
// 	case <-ctx.Done():
// 		return ctx.Err()
// 	default:
// 	}
// 	select {
// 	case <-ctx.Done():
// 		return ctx.Err()
// 	case <-s.signal:
// 		if s.available.Load() > 0 {
// 			s.available.Add(-1)
// 		}
// 		return nil
// 	default:
// 		if s.available.Load() > 0 {
// 			s.available.Add(-1)
// 			return nil
// 		}
// 	}
// 	select {
// 	case <-s.signal:
// 		s.available.Add(-1)
// 		return nil
// 	case <-ctx.Done():
// 		return ctx.Err()
// 	}
// }
