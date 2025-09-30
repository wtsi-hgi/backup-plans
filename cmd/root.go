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

// logToFile logs to the given file.
func logToFile(path string) {
	fh, err := log15.FileHandler(path, log15.LogfmtFormat())
	if err != nil {
		fh = log15.StderrHandler

		warn("can't write to log file; logging to stderr instead (%s)", err)
	}

	appLogger.SetHandler(fh)
}

func cliPrint(msg string) {
	fmt.Fprint(os.Stdout, msg)
}

// cliPrintf outputs the message to STDOUT.
func cliPrintf(msg string, a ...interface{}) {
	fmt.Fprintf(os.Stdout, msg, a...)
}

// cliPrintRaw is like cliPrint, but does no interpretation of placeholders in
// msg.
func cliPrintRaw(msg string) {
	fmt.Fprint(os.Stdout, msg)
}

// info is a convenience to log a message at the Info level.
func info(msg string, a ...interface{}) {
	appLogger.Info(fmt.Sprintf(msg, a...))
}

// warn is a convenience to log a message at the Warn level.
func warn(msg string, a ...interface{}) {
	appLogger.Warn(fmt.Sprintf(msg, a...))
}

// die is a convenience to log a message at the Error level and exit non zero.
func die(err error) {
	appLogger.Error(err.Error())
	os.Exit(1)
}

// die is a convenience to log a message at the Error level and exit non zero.
func dief(msg string, a ...interface{}) {
	appLogger.Error(fmt.Sprintf(msg, a...))
	os.Exit(1)
}
