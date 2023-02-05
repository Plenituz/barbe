# Installing barbe

Barbe relies on the Docker CLI and a container runtime to run. If you're not sure what that is, the script below will do everything you need. If you already have Docker (or any container runtime) installed, make sure it is running when using barbe.

## Mac

```bash
curl -fsSL https://hub.barbe.app/install.sh -o install-barbe.sh
sh install-barbe.sh

# check if it's working
barbe
```

## Linux

```bash
curl -fsSL https://hub.barbe.app/install.sh -o install-barbe.sh
sudo sh install-barbe.sh

# check if it's working
barbe
```

## Windows

On Windows, you'll have to install Docker Desktop and make sure it's running before using barbe.

```
Invoke-WebRequest -UseBasicParsing -Uri https://hub.barbe.app/install.ps1 | Invoke-Expression
```

This [Stack Overflow article](https://stackoverflow.com/questions/1618280/where-can-i-set-path-to-make-exe-on-windows) contains instructions for setting the PATH on Windows through the user interface.

You can check the binary is properly in your PATH by opening a new terminal and running
```bash
barbe
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