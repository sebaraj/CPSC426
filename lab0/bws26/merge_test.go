package lab0_test

import (
	"context"
	"strconv"
	"testing"

	"cs426.cloud/lab0"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func chanToSlice[T any](ch chan T) []T {
	vals := make([]T, 0)
	for item := range ch {
		vals = append(vals, item)
	}
	return vals
}

type mergeFunc = func(chan string, chan string, chan string)

func runMergeTest(t *testing.T, merge mergeFunc) {
	t.Run("empty channels", func(t *testing.T) {
		a := make(chan string)
		b := make(chan string)
		out := make(chan string)
		close(a)
		close(b)

		merge(a, b, out)
		// If your lab0 hangs here, make sure you are closing your channels!
		require.Empty(t, chanToSlice(out))
	})
	// TODO:   Please write your own tests
	t.Run("merge channels with different sizes", func(t *testing.T) {
		a := make(chan string, 3)
		b := make(chan string, 1)
		out := make(chan string, 10)

		a <- "a1"
		a <- "a2"
		a <- "a3"
		close(a)

		b <- "b1"
		close(b)

		lab0.MergeChannels(a, b, out)

		require.ElementsMatch(t, []string{"a1", "a2", "a3", "b1"}, chanToSlice(out))
	})

	t.Run("merge channels with different sizes output too small", func(t *testing.T) {
		a := make(chan string, 6)
		b := make(chan string, 4)
		const outSz int = 7
		out := make(chan string, outSz)
		b <- "b1"

		a <- "a1"
		a <- "a2"
		a <- "a3"
		a <- "a4"
		a <- "a5"
		a <- "a6"
		// close(a)

		b <- "b2"
		b <- "b3"
		b <- "b4"
		close(b)
		lab0.MergeChannels(a, b, out)
		a <- "a7"
		close(a)
		outSlice := chanToSlice(out)

		validElems := []string{"a1", "a2", "a3", "a4", "a5", "a6", "b1", "b2", "b3", "b4", "a7"}

		// goes through all 10 elems
		for _, elem := range outSlice {
			require.Contains(t, validElems, elem)
		}
		require.Equal(t, "", <-out)
	})

	t.Run("merge channels with large input", func(t *testing.T) {
		a := make(chan string, 1000)
		b := make(chan string, 1000)
		out := make(chan string, 2000)

		for i := 0; i < 1000; i++ {
			a <- ("a" + strconv.Itoa(i))
			b <- ("b" + strconv.Itoa(i))
		}
		close(a)
		close(b)

		merge(a, b, out)

		slice := chanToSlice(out)

		expectedSlice := make([]string, 2000)
		for i := 0; i < 1000; i++ {
			expectedSlice[2*i] = "a" + strconv.Itoa(i)
			expectedSlice[2*i+1] = "b" + strconv.Itoa(i)
		}

		require.Equal(t, 2000, len(slice))
		require.ElementsMatch(t, slice, expectedSlice)
	})
}

func TestMergeChannels(t *testing.T) {
	runMergeTest(t, func(a, b, out chan string) {
		lab0.MergeChannels(a, b, out)
	})
}

func TestMergeOrCancel(t *testing.T) {
	runMergeTest(t, func(a, b, out chan string) {
		_ = lab0.MergeChannelsOrCancel(context.Background(), a, b, out)
	})

	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		a := make(chan string, 1)
		b := make(chan string, 1)
		out := make(chan string, 10)

		eg, _ := errgroup.WithContext(context.Background())
		eg.Go(func() error {
			return lab0.MergeChannelsOrCancel(ctx, a, b, out)
		})
		err := eg.Wait()
		a <- "a"
		b <- "b"

		require.Error(t, err)
		require.Equal(t, []string{}, chanToSlice(out))
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		a := make(chan string)
		b := make(chan string)
		out := make(chan string, 10)

		eg, _ := errgroup.WithContext(context.Background())
		eg.Go(func() error {
			return lab0.MergeChannelsOrCancel(ctx, a, b, out)
		})
		a <- "a"
		b <- "b"

		// go func() {
		// 	a <- "a2"
		// }()

		cancel()

		err := eg.Wait()
		require.Error(t, err)
		require.ElementsMatch(t, []string{"a", "b"}, chanToSlice(out))
	})

	t.Run("merge or cancel immediate cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		a := make(chan string, 1)
		b := make(chan string, 1)
		out := make(chan string, 10)

		a <- "a"

		err := lab0.MergeChannelsOrCancel(ctx, a, b, out)
		require.Error(t, err)
		require.Equal(t, []string{}, chanToSlice(out))
	})
}

type channelFetcher struct {
	ch chan string
}

func newChannelFetcher(ch chan string) *channelFetcher {
	return &channelFetcher{ch: ch}
}

func (f *channelFetcher) Fetch() (string, bool) {
	v, ok := <-f.ch
	return v, ok
}

func TestMergeFetches(t *testing.T) {
	runMergeTest(t, func(a, b, out chan string) {
		lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)
	})
}

func TestMergeFetchesAdditional(t *testing.T) {
	// TODO: add your extra tests here
	t.Run("merge fetches with different finish time", func(t *testing.T) {
		a := make(chan string, 3)
		b := make(chan string, 1)
		out := make(chan string, 10)

		fetcherA := newChannelFetcher(a)
		fetcherB := newChannelFetcher(b)

		a <- "a1"
		a <- "a2"
		a <- "a3"
		close(a)

		b <- "b1"
		close(b)

		lab0.MergeFetches(fetcherA, fetcherB, out)

		require.ElementsMatch(t, []string{"a1", "a2", "a3", "b1"}, chanToSlice(out))
	})

	t.Run("merge fetches both input channels finish immediately", func(t *testing.T) {
		a := make(chan string)
		b := make(chan string)
		out := make(chan string)

		close(a)
		close(b)

		lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)
		require.Empty(t, chanToSlice(out))
	})

	t.Run("merge fetches but one input finishes immediately", func(t *testing.T) {
		a := make(chan string)
		b := make(chan string, 3)
		out := make(chan string, 3)

		close(a)
		b <- "b1"
		b <- "b2"
		close(b)

		lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)
		require.Equal(t, []string{"b1", "b2"}, chanToSlice(out))
	})
}
