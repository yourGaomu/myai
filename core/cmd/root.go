package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:           "myai",
	Short:         "A tiny AI coding CLI",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}
