// Package concurrent provides tiny fan-out primitives for running independent
// I/O calls (adb shell-outs, HTTP fetches) in parallel instead of one after
// another. It has no knowledge of adb, HTTP, or any other caller-specific
// concern — callers write their own result into a variable they own via
// closure capture, so RunAll/RunIndexed only need to own the join.
package concurrent

import "sync"

// RunAll runs each fn concurrently and blocks until every one has returned.
// It exists for calls that only depend on their own inputs — never on each
// other's output — so each fn is expected to write its own result via
// closure capture over a variable declared by the caller. Safe whenever the
// underlying calls share no mutable state (fresh buffers/requests per call).
func RunAll(fns ...func()) {
	RunIndexed(len(fns), func(i int) { fns[i]() })
}

// RunIndexed runs fn(i) concurrently for every i in [0,n) and blocks until
// all have returned. Use this instead of RunAll when a result needs to land
// at a caller-visible index — e.g. preserving iteration order across N
// independent per-item calls that would otherwise race to append.
func RunIndexed(n int, fn func(i int)) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}
