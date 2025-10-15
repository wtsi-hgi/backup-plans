package cmd

import (
	"fmt"
	"os"

	"github.com/inconshreveable/log15"
	"github.com/spf13/cobra"
)

// appLogger is used for logging events in our commands.
var appLogger = log15.New()

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "backup-plans",
	Short: "backup-plans is used to do all backup plans",
	Long:  `backup-plans is used to do all backup plans`,
}

// Execute adds all child commands to the root command and sets flags
// appropriately. This is called by main.main(). It only needs to happen once to
// the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		die(err)
	}
}

func init() {
	// set up logging to stderr
	appLogger.SetHandler(log15.LvlFilterHandler(log15.LvlInfo, log15.StderrHandler))
}

// cliPrintf outputs the message to STDOUT.
func cliPrintf(msg string, a ...interface{}) {
	fmt.Fprintf(os.Stdout, msg, a...) // nolint:errcheck
}

// die is a convenience to log a message at the Error level and exit non zero.
func die(err error) {
	appLogger.Error(err.Error()) // nolint:errcheck
	os.Exit(1)
}
