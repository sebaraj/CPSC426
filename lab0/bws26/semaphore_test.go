package lab0_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"cs426.cloud/lab0"

	"github.com/stretchr/testify/require"
)

func TestSemaphore(t *testing.T) {
	t.Run("semaphore basic", func(t *testing.T) {
		s := lab0.NewSemaphore()
		go func() {
			s.Post()
		}()
		err := s.Wait(context.Background())
		require.NoError(t, err)
	})
	t.Run("semaphore starts with zero available resources", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err := s.Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
	t.Run("semaphore post before wait does not block", func(t *testing.T) {
		s := lab0.NewSemaphore()
		s.Post()
		s.Post()
		s.Post()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := s.Wait(ctx)
		require.NoError(t, err)
	})
	t.Run("post after wait releases the wait", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(20 * time.Millisecond)
			s.Post()
		}()
		err := s.Wait(ctx)
		require.NoError(t, err)
	})
	t.Run("multiple waiters release in sequence (modified for semaphore with non-blocking release/post)", func(t *testing.T) {
		s := lab0.NewSemaphore()
		var err error
		var err1 error
		var err2 error
		done := make(chan struct{})
		var sum int64
		sum = 0
		go func() {
			for {
				if sum == 9 {
					close(done)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()

		go func(sum *int64) {
			err = s.Wait(context.Background())
			*sum += 1
			println("sum: ", *sum)
			// atomic.AddInt64(sum, 1)
			s.Post()
		}(&sum)

		go func(sum *int64) {
			err1 = s.Wait(context.Background())
			*sum += 3
			println("sum: ", *sum)
			// atomic.AddInt64(sum, 1)
			s.Post()
		}(&sum)

		go func(sum *int64) {
			err2 = s.Wait(context.Background())
			*sum += 5
			println("sum: ", *sum)
			// atomic.AddInt64(sum, 1)
			s.Post()
		}(&sum)
		s.Post()
		<-done
		err3 := s.Wait(context.Background())
		// err = s.Wait(context.Background())
		// err1 = s.Wait(context.Background())
		// err2 = s.Wait(context.Background())
		require.Equal(t, atomic.LoadInt64(&sum), int64(9))
		require.NoError(t, err)
		require.NoError(t, err1)
		require.NoError(t, err2)
		require.NoError(t, err3)
	})

	t.Run("test that s.Post() is non-blocking)", func(t *testing.T) {
		s := lab0.NewSemaphore()

		// Post multiple times without any waiters
		s.Post()
		s.Post()
		s.Post()
		s.Post()

		go func() {
			s.Wait(context.Background())
			s.Post()
		}()

		err := s.Wait(context.Background())
		require.NoError(t, err)
	})

	t.Run("wait with negative timeout context", func(t *testing.T) {
		s := lab0.NewSemaphore()

		ctx, cancel := context.WithTimeout(context.Background(), -1*time.Millisecond)
		defer cancel()

		err := s.Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("tests that s.Wait() is blocking)", func(t *testing.T) {
		s := lab0.NewSemaphore()

		s.Post()
		s.Post()
		s.Post()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		s.Wait(context.Background())
		s.Wait(context.Background())
		err := s.Wait(context.Background())
		require.NoError(t, err)

		err = s.Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("test that ", func(t *testing.T) {
		s := lab0.NewSemaphore()
		var errs [3]error
		results := make(chan struct {
			index int
			err   error
		}, 3)

		for i := 0; i < 3; i++ {
			go func(i int) {
				err := s.Wait(context.Background())
				results <- struct {
					index int
					err   error
				}{index: i, err: err}
			}(i)
		}

		time.Sleep(50 * time.Millisecond)

		s.Post()
		s.Post()
		s.Post()

		for i := 0; i < 3; i++ {
			result := <-results
			errs[result.index] = result.err
		}

		for i := 0; i < 3; i++ {
			require.NoError(t, errs[i], "goroutine %d encountered an unexpected error", i)
		}
	})

	t.Run("test that 2", func(t *testing.T) {
		s := lab0.NewSemaphore()

		go func() {
			time.Sleep(20 * time.Millisecond)
			s.Post()
			time.Sleep(20 * time.Millisecond)
			s.Post()
			time.Sleep(20 * time.Millisecond)
			s.Post()
			time.Sleep(20 * time.Millisecond)
			s.Post()
		}()
		err1 := s.Wait(context.Background())
		err2 := s.Wait(context.Background())
		err3 := s.Wait(context.Background())

		err4 := s.Wait(context.Background())

		require.NoError(t, err1, "Wait 1 encountered an unexpected error")
		require.NoError(t, err2, "Wait 2 encountered an unexpected error")
		require.NoError(t, err3, "Wait 3 encountered an unexpected error")
		require.NoError(t, err4, "Wait 4 encountered an unexpected error")
	})
}
