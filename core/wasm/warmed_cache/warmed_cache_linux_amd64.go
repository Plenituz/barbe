//go:build linux && amd64

package warmed_cache

import (
	"barbe/core/wasm"
	"embed"
)

//go:embed wazero-v1.0.0-pre.9-amd64-linux
var WarmedCache embed.FS

func init() {
	wasm.WarmedCache = WarmedCache
}
