package main

import (
	"os"

	"github.com/ivan4th/ameriagrab/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
