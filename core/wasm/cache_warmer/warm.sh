set -e
set -x

rm -rf cache
go run main.go
cp -r cache/* ../warmed_cache/.