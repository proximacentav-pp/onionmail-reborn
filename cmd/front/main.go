package main

import (
	"fmt"
	"os"

	"onionmail/internal/front"
)

func main() {
	s, err := front.NewServer()
	if err != nil {
		fmt.Fprintln(os.Stderr, "front:", err)
		os.Exit(1)
	}
	if err := s.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, "front:", err)
		os.Exit(1)
	}
}
