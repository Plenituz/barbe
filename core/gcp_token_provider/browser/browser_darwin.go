package browser

func openBrowser(url string, cmdOptions []CmdOption) error {
	return runCmd("open", []string{url}, cmdOptions)
}
