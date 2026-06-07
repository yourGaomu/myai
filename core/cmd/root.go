package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "myai",
	Short: "A tiny AI coding CLI",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Hello from myai")
	},
}

func Execute() error {
	return rootCmd.Execute()
}
