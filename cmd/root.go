package cmd

import (
	"log"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "miadeploy",
	Short: "Miadeploy provides utilities to run Mia Platform pipelines.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
