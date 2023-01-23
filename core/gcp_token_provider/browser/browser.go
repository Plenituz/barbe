// Package browser provides helpers to open files, readers, and urls in a browser window.
//
// The choice of which browser is started is entirely client dependant.
package browser

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

type CmdOption func(*exec.Cmd)

// Stdout is the io.Writer to which executed commands write standard output.
var Stdout io.Writer = os.Stdout

// Stderr is the io.Writer to which executed commands write standard error.
var Stderr io.Writer = os.Stderr

// OpenFile opens new browser window for the file path.
func OpenFile(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return OpenURL("file://" + path)
}

// OpenReader consumes the contents of r and presents the
// results in a new browser window.
func OpenReader(r io.Reader) error {
	f, err := ioutil.TempFile("", "browser.*.html")
	if err != nil {
		return fmt.Errorf("browser: could not create temporary file: %v", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return fmt.Errorf("browser: caching temporary file failed: %v", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("browser: caching temporary file failed: %v", err)
	}
	return OpenFile(f.Name())
}

// OpenURL opens a new browser window pointing to url.
func OpenURL(url string, options ...CmdOption) error {
	return openBrowser(url, options)
}

func runCmd(prog string, args []string, options []CmdOption) error {
	cmd := exec.Command(prog, args...)
	cmd.Stdout = Stdout
	cmd.Stderr = Stderr
	for _, o := range options {
		o(cmd)
	}
	return cmd.Run()
}
