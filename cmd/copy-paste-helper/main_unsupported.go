//go:build !linux && !windows

// Command copy-paste-helper is buildable on unsupported systems so its shared
// packages can still be tested there.
package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintf(os.Stderr, "copy-paste-helper: global keyboard capture is not supported on %s\n", runtime.GOOS)
	os.Exit(1)
}
