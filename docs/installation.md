# Installing barbe

There is currently no official release of barbe, so you will need to build it yourself. 

Thankfully this isn't too hard for Go projects, you just need to have [Go](https://golang.org/doc/install) installed on your machine.

```bash
git clone https://github.com/Plenituz/barbe.git
cd barbe
make barbe
# if you dont have make installed, you can run the following instead
# cd cli && go build -o barbe main.go && cd -

# link the binary to your path
# linux
ln -s `pwd`/cli/barbe /usr/local/bin/barbe
# mac intel
ln -s `pwd`/cli/barbe /usr/local/bin/barbe
# mac ARM
ln -s `pwd`/cli/barbe /opt/homebrew/bin/barbe
```
