module barbe/cli

go 1.16

replace barbe/core => ../core

require (
	barbe/core v0.0.0-00010101000000-000000000000
	github.com/mattn/go-colorable v0.1.13
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db
	github.com/rs/zerolog v1.27.0
	github.com/spf13/cobra v1.5.0
	github.com/spf13/viper v1.12.0
	golang.org/x/term v0.0.0-20220722155259-a9ba230a4035
)
