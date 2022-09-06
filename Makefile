
.PHONY: barbe
barbe:
	cd cli && CGO_ENABLED=0 go build -o barbe -ldflags '-s -w' main.go
	echo "built at `pwd`/cli/barbe"