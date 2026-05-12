package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := runMain(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
