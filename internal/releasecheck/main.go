package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nightingale-develop/reconcile-guard/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "Usage: go run ./internal/releasecheck <release-tag>")
		return 1
	}
	expected := "v" + version.Current
	if args[0] != expected {
		fmt.Fprintf(stderr, "Release tag %q does not match application version %q; expected %q.\n", args[0], version.Current, expected)
		return 1
	}
	if _, err := fmt.Fprintf(stdout, "Release tag %s matches application version %s.\n", args[0], version.Current); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}
