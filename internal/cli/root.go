package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "portless",
	Short: "Portless Dev Router - A lightweight local service router",
	Long: `Portless is a developer tool that eliminates port conflicts 
by allowing you to hit clean internal domains (*.internal) instead of localhost:PORT.

Documentation is available at https://github.com/ivin-titus/portless`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}
