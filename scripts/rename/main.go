// rename is a portable cross-platform replacement for `mv src dst`.
// Usage: go run tools/rename/main.go <src> <dst>
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: rename <src> <dst>")
		os.Exit(1)
	}
	if err := os.Rename(os.Args[1], os.Args[2]); err != nil { //nolint:gosec // intentional: tool accepts user-supplied paths by design
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
