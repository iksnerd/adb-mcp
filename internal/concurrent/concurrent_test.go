package concurrent

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestRunAllRunsConcurrently proves RunAll actually parallelizes instead of
// just being a fancy sequential loop: N fns that each sleep for delay must
// together take close to one delay, not delay*N.
func TestRunAllRunsConcurrently(t *testing.T) {
	const n = 5
	const delay = 60 * time.Millisecond

	fns := make([]func(), n)
	for i := range fns {
		fns[i] = func() { time.Sleep(delay) }
	}

	start := time.Now()
	RunAll(fns...)
	elapsed := time.Since(start)

	if elapsed >= delay*n {
		t.Fatalf("RunAll took %s, looks sequential (n*delay = %s)", elapsed, delay*n)
	}
	if elapsed < delay {
		t.Fatalf("RunAll returned after %s, before a single delay (%s) elapsed", elapsed, delay)
	}
}

// TestRunAllWaitsForEveryFn guards the join itself: RunAll must not return
// until every fn — not just the fastest ones — has completed.
func TestRunAllWaitsForEveryFn(t *testing.T) {
	const n = 20
	var done atomic.Int32
	fns := make([]func(), n)
	for i := range fns {
		fns[i] = func() { done.Add(1) }
	}

	RunAll(fns...)

	if got := done.Load(); got != n {
		t.Fatalf("done=%d after RunAll returned, want %d", got, n)
	}
}

// TestRunIndexedPreservesOrder proves that even when later indices finish
// first, each result still lands at its own caller-visible index — the whole
// reason to use RunIndexed over an unordered fan-out.
func TestRunIndexedPreservesOrder(t *testing.T) {
	const n = 8
	results := make([]int, n)

	RunIndexed(n, func(i int) {
		// Reverse the completion order relative to index so a naive
		// append-as-you-go implementation would visibly scramble results.
		time.Sleep(time.Duration(n-i) * 5 * time.Millisecond)
		results[i] = i * i
	})

	for i, got := range results {
		if want := i * i; got != want {
			t.Errorf("results[%d] = %d, want %d", i, got, want)
		}
	}
}

// TestRunIndexedZero guards the empty-input edge case: no goroutines, no
// hang, immediate return.
func TestRunIndexedZero(t *testing.T) {
	called := false
	RunIndexed(0, func(int) { called = true })
	if called {
		t.Fatal("fn should never run for n=0")
	}
}
