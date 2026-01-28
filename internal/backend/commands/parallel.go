package commands

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// parallelFor runs fn(y) over y in [0, n) using up to GOMAXPROCS workers.
// Work is distributed by striding to balance uneven workloads.
func parallelFor(n int, fn func(y int)) {
	// Implemented via parallelForStop to avoid code duplication
	_ = parallelForStop(n, func(y int) bool {
		fn(y)
		return false
	})
}

// parallelForStop runs fn(y) over y in [0, n) using up to GOMAXPROCS workers.
// If any fn invocation returns true, all workers stop early and the function returns true.
// Returns false if all work completed without any fn returning true.
func parallelForStop(n int, fn func(y int) bool) bool {
	if n <= 0 {
		return false
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > n {
		workers = n
	}

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		w := w
		go func() {
			defer wg.Done()
			for y := w; y < n && !stop.Load(); y += workers {
				if fn(y) {
					stop.Store(true)
					return
				}
			}
		}()
	}

	wg.Wait()
	return stop.Load()
}
