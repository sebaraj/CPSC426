package lab0

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// ParallelFetcher manages concurrent fetches of resources that the underlying Fetcher interacts with.
// The ParallelFetcher imposes an upper limit allowed on the number of concurrent (and parallel) fetches.
//
// You can use a `semaphore.Weighted` with `context.Background()` to handle the blocking.
type ParallelFetcher struct {
	fetcher Fetcher
	// Add your fields here
	sema      *semaphore.Weighted
	callAgain bool
}

// ParallelFetcher ensures that no more than maxConcurrentLimit clients call `Fetcher.Fetch()` at any given time.
// Additional concurrent calls to `ParallelFetcher.Fetch()` should block until the underlying Fetcher
// becomes available (i.e., one of the previous Fetcher.Fetch() finishes).
//
// You may assume the underlying `Fetcher.Fetch()` is thread-safe.
func NewParallelFetcher(fetcher Fetcher, maxConcurrencyLimit int) *ParallelFetcher {
	return &ParallelFetcher{
		fetcher: fetcher,
		// Add more initialization here
		sema:      semaphore.NewWeighted(int64(max(maxConcurrencyLimit, 1))),
		callAgain: true,
	}
}

// Addendum to the `Fetcher.Fetch()` contract: Fetch() should not be called again
// once `false` is returned; *however*, it is OK to have Fetch()s that are already in progress
// (which will also return false).
func (pf *ParallelFetcher) Fetch() (string, bool) {
	// Add your implementation here
	if !pf.callAgain {
		return "", false
	}
	if err := pf.sema.Acquire(context.Background(), 1); err != nil {
		return "", false
	}
	defer pf.sema.Release(1)
	s, more := pf.fetcher.Fetch()
	if !more {
		pf.callAgain = false
	}
	return s, more
}
