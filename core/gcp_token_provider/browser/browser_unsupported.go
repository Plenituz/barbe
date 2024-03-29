//go:build !linux && !windows && !darwin && !openbsd && !freebsd && !netbsd
// +build !linux,!windows,!darwin,!openbsd,!freebsd,!netbsd

package browser

import (
	"fmt"
	"runtime"
)

func openBrowser(url string, cmdOptions []CmdOption) error {
	return fmt.Errorf("openBrowser: unsupported operating system: %v", runtime.GOOS)
}
