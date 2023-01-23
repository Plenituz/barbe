// stole this from dagger for now

package logger

import (
	"barbe/core/state_display"
	"context"
	"fmt"
	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/term"
	"os"
	"strings"
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

func PromptUserYesNo(ctx context.Context, msg string) (bool, error) {
	if viper.GetBool("auto-approve") {
		return true, nil
	}
	resp, err := PromptUser(ctx, msg+" [yes/NO]")
	if err != nil {
		return false, err
	}
	return strings.ToLower(resp) == "yes", nil
}

func PromptUser(ctx context.Context, msg string) (string, error) {
	if jsonLogs() || viper.GetBool("no-input") {
		return "", fmt.Errorf("cannot prompt user in json mode")
	}
	if viper.GetString("log-format") == "plain" {
		log.Ctx(ctx).Info().Msg(msg)
		var resp string
		_, err := fmt.Scanln(&resp)
		return resp, err
	} else {
		state_display.GlobalState.PromptUser(&msg)

		var resp string
		_, err := fmt.Scanln(&resp)
		if err != nil {
			return "", err
		}
		state_display.GlobalState.PromptUser(nil)
		return resp, nil
	}
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
