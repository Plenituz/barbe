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

func New() zerolog.Logger {
	logger := zerolog.
		New(os.Stderr).
		With().
		Timestamp().
		Logger()

	if !jsonLogs() {
		logger = logger.Output(&PlainOutput{Out: colorable.NewColorableStderr()})
	}

	level := viper.GetString("log-level")
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		panic(err)
	}
	return logger.Level(lvl)
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
