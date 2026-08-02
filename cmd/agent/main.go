package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"horizonx/internal/app"
)

func main() {
	_ = godotenv.Load()

	if err := app.RunAgent(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}
