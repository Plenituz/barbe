# Installing barbe

To install Barbe, find the appropriate package for your system and download it as a zip archive from the [list of releases](https://github.com/Plenituz/barbe/releases)

After downloading Barbe, unzip the package and place the binary on your `PATH`. This process will depend on your operating system.

Barbe also relies on `docker` to securely execute commands inside containers. Make sure you have [`docker` installed](https://docs.docker.com/get-docker/) and running on your system.

## Max/Linux

You can check the value of your `PATH` variable by running `echo $PATH` in your terminal. Then you can move the Barbe binary to a directory that is in your `PATH` variable using
```bash
mv ~/Downloads/barbe /usr/local/bin/
```

On Mac you will have to either authorize the binary, or [build from source](#building-from-source) yourself for now

Also see [this Stack Overflow article](https://stackoverflow.com/questions/14637979/how-to-permanently-set-path-on-linux-unix).

## Windows

This [Stack Overflow article](https://stackoverflow.com/questions/1618280/where-can-i-set-path-to-make-exe-on-windows) contains instructions for setting the PATH on Windows through the user interface.


## Verifying the installation

You can check the binary is properly in your PATH by opening a new terminal and running
```bash
$ barbe
# A programmable syntax manipulation engine
# 
# Usage:
#   barbe [command]
# 
# Available Commands:
#   apply       Generate files based on the given configuration, and execute all the appliers that will deploy the generated files
#   generate    Generate files based on the given configuration
#   help        Help about any command
#   version     Print version
# 
# Flags:
#   -h, --help                help for barbe
#       --log-format string   Log format (auto, plain, json) (default "auto")
#   -l, --log-level string    Log level (default "info")
#   -o, --output string       Output directory (default ".")
# 
# Use "barbe [command] --help" for more information about a command.
```


# Building from source

To build Barbe from source you will need [Go](https://golang.org/doc/install) installed on your system.

```bash
git clone https://github.com/Plenituz/barbe.git
cd barbe

# To build just for your system
make barbe-dev

# To build for all systems
make barbe
```