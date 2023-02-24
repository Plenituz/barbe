package main

import (
	"barbe/cli/cmd"
	_ "barbe/core/wasm/warmed_cache"
)

func main() {
	cmd.Execute()
}
