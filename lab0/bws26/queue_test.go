package lab0_test

import (
	"context"
	"sync/atomic"
	"testing"

	"cs426.cloud/lab0"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

type intQueue interface {
	Push(i int)
	Pop() (int, bool)
}

type stringQueue interface {
	Push(s string)
	Pop() (string, bool)
}

func runIntQueueTests(t *testing.T, q intQueue) {
	t.Run("queue starts empty", func(t *testing.T) {
		_, ok := q.Pop()
		require.False(t, ok)
	})

	t.Run("simple push pop", func(t *testing.T) {
		q.Push(1)
		q.Push(2)
		q.Push(3)

		x, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, 1, x)

		x, ok = q.Pop()
		require.True(t, ok)
		require.Equal(t, 2, x)

		x, ok = q.Pop()
		require.True(t, ok)
		require.Equal(t, 3, x)
	})

	t.Run("queue empty again", func(t *testing.T) {
		_, ok := q.Pop()
		require.False(t, ok)
	})

	t.Run("push and pop", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			q.Push(i)
		}
		for i := 1; i <= 5; i++ {
			x, ok := q.Pop()
			require.True(t, ok)
			require.Equal(t, i, x)
		}

		for i := 11; i <= 100; i++ {
			q.Push(i)
		}

		x, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, 6, x)

		q.Push(0)
		for i := 7; i <= 100; i++ {
			x, ok := q.Pop()
			require.True(t, ok)
			require.Equal(t, i, x)
		}
		x, ok = q.Pop()
		require.True(t, ok)
		require.Equal(t, 0, x)

		_, ok = q.Pop()
		require.False(t, ok)

		_, ok = q.Pop()
		require.False(t, ok)
	})

	// TODO: more queue int tests
	t.Run("push nil to the queue", func(t *testing.T) {
		qp := lab0.NewQueue[*int]()
		require.NotNil(t, qp)
		qp.Push(nil)

		val, ok := qp.Pop()
		require.True(t, ok)
		require.Nil(t, val)
	})

	t.Run("pop beyond empty after push", func(t *testing.T) {
		q.Push(1)
		q.Push(2)

		for i := 0; i < 10; i++ {
			val, ok := q.Pop()
			if i < 2 {
				require.True(t, ok)
				require.NotNil(t, val)
			} else {
				require.False(t, ok)
			}
		}
	})

	t.Run("large number of elements", func(t *testing.T) {
		for i := 0; i < 1000000; i++ {
			q.Push(i)
		}
		for i := 0; i < 1000000; i++ {
			val, ok := q.Pop()
			require.True(t, ok)
			require.Equal(t, i, val)
		}
		_, ok := q.Pop()
		require.False(t, ok)
	})

	t.Run("push after popping everything", func(t *testing.T) {
		_, ok := q.Pop()
		require.False(t, ok)

		q.Push(100)

		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, 100, val)

		_, ok = q.Pop()
		require.False(t, ok)
	})
}

func runStringQueueTests(t *testing.T, q stringQueue) {
	t.Run("with strings", func(t *testing.T) {
		_, ok := q.Pop()
		require.False(t, ok)

		q.Push("hello!")

		v, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, "hello!", v)
	})
}

func runStringQueueEdgeCaseTests(t *testing.T, q stringQueue) {
	t.Run("queue starts empty", func(t *testing.T) {
		_, ok := q.Pop()
		require.False(t, ok)
	})

	t.Run("empty string push and pop", func(t *testing.T) {
		q.Push("")
		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, "", val)
	})

	t.Run("large string", func(t *testing.T) {
		largeString := "a" + string(make([]byte, 1000000)) + "z"
		q.Push(largeString)
		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, largeString, val)
	})

	t.Run("push after popping everything", func(t *testing.T) {
		q.Push("test string")
		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, "test string", val)

		_, ok = q.Pop()
		require.False(t, ok)

		q.Push("another string")
		val, ok = q.Pop()
		require.True(t, ok)
		require.Equal(t, "another string", val)
	})

	t.Run("unicode string", func(t *testing.T) {
		unicodeString := "丁丁, 丏丏, 丑丑, 丒丒, 专专, 且且, 丕丕, 世世"
		q.Push(unicodeString)
		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, unicodeString, val)
	})

	t.Run("nerdfont string", func(t *testing.T) {
		unicodeString := "󱡅, 󱡅"
		q.Push(unicodeString)
		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, unicodeString, val)
	})

	t.Run("strings with special characters", func(t *testing.T) {
		specialCharString := "Hello, World! @#$%^&*()_+"
		q.Push(specialCharString)
		val, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, specialCharString, val)
	})
}

func runConcurrentQueueTests(t *testing.T, q *lab0.ConcurrentQueue[int]) {
	const concurrency = 16
	const pushes = 1000

	ctx := context.Background()
	t.Run("concurrent pushes", func(t *testing.T) {
		eg, _ := errgroup.WithContext(ctx)
		for i := 0; i < concurrency; i++ {
			eg.Go(func() error {
				for i := 0; i < pushes; i++ {
					q.Push(1234)
				}
				return nil
			})
		}
		eg.Wait()

		for i := 0; i < concurrency*pushes; i++ {
			v, ok := q.Pop()
			require.True(t, ok)
			require.Equal(t, 1234, v)
		}
		_, ok := q.Pop()
		require.False(t, ok)
	})

	t.Run("concurrent pops", func(t *testing.T) {
		eg, _ := errgroup.WithContext(ctx)
		for i := 0; i < concurrency*pushes/2; i++ {
			q.Push(i)
		}

		var sum int64
		var found int64
		for i := 0; i < concurrency; i++ {
			eg.Go(func() error {
				for i := 0; i < pushes; i++ {
					v, ok := q.Pop()
					if ok {
						atomic.AddInt64(&sum, int64(v))
						atomic.AddInt64(&found, 1)
					}
				}
				return nil
			})
		}
		eg.Wait()
		// In the end, we should have exactly concurrency * pushes/2 elements in popped
		require.Equal(t, int64(concurrency*pushes/2), found)
		// Mathematical SUM(i=0...7999)
		require.Equal(t, int64(31996000), sum)
	})

	t.Run("concurrent pop when empty", func(t *testing.T) {
		const concurrency int = 10
		var popsSuccess int32
		eg, _ := errgroup.WithContext(context.Background())
		for i := 0; i < concurrency; i++ {
			eg.Go(func() error {
				_, ok := q.Pop()
				if ok {
					atomic.AddInt32(&popsSuccess, 1)
				}
				return nil
			})
		}
		eg.Wait()
		require.Equal(t, int32(0), popsSuccess)
	})

	t.Run("concurrent push after empty", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			q.Push(i)
		}
		for i := 0; i < 10; i++ {
			_, ok := q.Pop()
			require.True(t, ok)

		}
		const concurrency int = 5
		eg, _ := errgroup.WithContext(context.Background())
		for i := 0; i < concurrency; i++ {
			eg.Go(func() error {
				q.Push(100)
				return nil
			})
		}
		eg.Wait()

		for i := 0; i < concurrency; i++ {
			val, ok := q.Pop()
			require.True(t, ok)
			require.Equal(t, 100, val)
		}

		_, ok := q.Pop()
		require.False(t, ok)
	})

	t.Run("high load stress test", func(t *testing.T) {
		ctx := context.Background()

		egPushers, _ := errgroup.WithContext(ctx)
		for i := 0; i < 100; i++ {
			egPushers.Go(func() error {
				for j := 0; j < 100000; j++ {
					q.Push(j)
				}
				return nil
			})
		}

		require.NoError(t, egPushers.Wait())

		egPoppers, _ := errgroup.WithContext(ctx)
		var poppedCount int64 = 0
		for i := 0; i < 100; i++ {
			egPoppers.Go(func() error {
				for {
					_, ok := q.Pop()
					if ok {
						atomic.AddInt64(&poppedCount, 1)
					} else {
						return nil
					}
				}
			})
		}

		require.NoError(t, egPoppers.Wait())

		require.Equal(t, int64(10000000), poppedCount)
	})
}

func TestGenericQueue(t *testing.T) {
	q := lab0.NewQueue[int]()

	require.NotNil(t, q)
	runIntQueueTests(t, q)

	qs := lab0.NewQueue[string]()
	require.NotNil(t, qs)
	runStringQueueTests(t, qs)
	runStringQueueEdgeCaseTests(t, qs)
}

func TestConcurrentQueue(t *testing.T) {
	q := lab0.NewConcurrentQueue[int]()
	require.NotNil(t, q)
	runIntQueueTests(t, q)

	qs := lab0.NewConcurrentQueue[string]()
	require.NotNil(t, qs)
	runStringQueueTests(t, qs)

	qc := lab0.NewConcurrentQueue[int]()
	require.NotNil(t, qc)
	runConcurrentQueueTests(t, qc)
}
