package main

import (
	"fmt"
	"os"

	"github.com/icarus-itcs/lazycap/cmd/lazycap"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := lazycap.Execute(version, commit, date); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
