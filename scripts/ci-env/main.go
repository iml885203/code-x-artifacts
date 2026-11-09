// Command ci-env prints the CPU facts Go sees, for scripts/ci-env.sh.
package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	_, _ = fmt.Fprintf(os.Stdout, "NumCPU=%d GOMAXPROCS=%d %s\n", runtime.NumCPU(), runtime.GOMAXPROCS(0), runtime.Version())
}
