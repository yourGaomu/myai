package main

import (
	"fmt"
	"os"

	"myai/core/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
