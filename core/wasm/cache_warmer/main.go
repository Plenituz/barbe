package main

import (
	"context"
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"os"
)

func main() {
	ctx := context.Background()

	spiderMonkey, err := os.ReadFile("../js.wasm")
	if err != nil {
		panic(err)
	}

	cache, err := wazero.NewCompilationCacheWithDir("./cache")
	if err != nil {
		panic(err)
	}
	compiledRuntime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCompilationCache(cache))
	wasi_snapshot_preview1.MustInstantiate(ctx, compiledRuntime)

	_, err = compiledRuntime.CompileModule(ctx, spiderMonkey)
	if err != nil {
		panic(err)
	}
	fmt.Println("done compiling spidermonkey")
}
