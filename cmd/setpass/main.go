// Command setpass prints a bcrypt hash for the given password.
// Usage: go run ./cmd/setpass 'newpass'
package main

import (
	"fmt"
	"os"

	"hugostack/internal/auth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: setpass <password>")
		os.Exit(1)
	}
	h, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(h)
}
