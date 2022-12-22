// stole this from dagger for now

package logger

import (
	"fmt"
	"github.com/mattn/go-colorable"
	"github.com/spf13/viper"
	"os"

	"github.com/rs/zerolog"
	"golang.org/x/term"
)

func New() (zerolog.Logger, func()) {
	level := viper.GetString("log-level")
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		panic(err)
	}
	logger := zerolog.
		New(os.Stderr).
		With().
		Timestamp().
		Logger().
		Level(lvl)

	if !jsonLogs() {
		if viper.GetString("log-format") == "plain" {
			logger = logger.Output(&PlainOutput{Out: colorable.NewColorableStderr()})
		} else {
			logger = logger.Output(NewFancyOutput())
			closer := StartFancyDisplay(logger)
			return logger, closer
		}
	}
	return logger, func() {}
}

func jsonLogs() bool {
	switch f := viper.GetString("log-format"); f {
	case "json":
		return true
	case "plain":
		return false
	case "auto":
		return !term.IsTerminal(int(os.Stdout.Fd()))
	default:
		fmt.Fprintf(os.Stderr, "invalid --log-format %q\n", f)
		os.Exit(1)
	}
	return false
}
