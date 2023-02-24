//go:build darwin && amd64

package warmed_cache

import (
	"barbe/core/wasm"
	"embed"
)

//go:embed warmed_cache_darwin_amd64.go
var WarmedCache embed.FS

func init() {
	wasm.WarmedCache = WarmedCache
}
