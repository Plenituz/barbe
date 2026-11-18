package buildkit_runner

import (
	"context"
	"os"
	"testing"

	"barbe/core"
	"github.com/moby/buildkit/solver/pb"
)

func TestNamedSourcePaths(t *testing.T) {
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Error(err)
		}
	})
	if err := os.WriteFile(".dockerignore", []byte("# rules\nbin/*\n!bin/worker\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		paths        []string
		wantLocal    bool
		wantPaths    string
		wantExcludes string
	}{
		{name: "default", wantLocal: true},
		{name: "generated files only", paths: []string{}},
		{name: "one module", paths: []string{"./barbe_dist/ecr"}, wantLocal: true, wantPaths: `["barbe_dist/ecr"]`},
		{name: "image ignore rules", wantLocal: true, wantExcludes: `["bin/*","!bin/worker"]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := map[string]interface{}{
				"dockerfile": "FROM scratch\nCOPY --from=src / /",
				"input_files": map[string]interface{}{
					"generated": "from component",
				},
			}
			if test.paths != nil {
				input["source_paths"] = test.paths
			}
			if test.wantExcludes != "" {
				input["use_root_dockerignore"] = true
			}
			token, err := core.GoValueToToken(input)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			config, err := parseRunnerConfig(ctx, token.ObjectConst)
			if err != nil {
				t.Fatal(err)
			}
			state, err := buildLlbDefinition(ctx, config, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			def, err := state.Marshal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var local, generated bool
			for _, data := range def.Def {
				var op pb.Op
				if err := op.Unmarshal(data); err != nil {
					t.Fatal(err)
				}
				if src := op.GetSource(); src != nil && src.Identifier == "local://src" {
					local = true
					if got := src.Attrs[pb.AttrFollowPaths]; got != test.wantPaths {
						t.Errorf("local follow paths = %q, want %q", got, test.wantPaths)
					}
					if got := src.Attrs[pb.AttrExcludePatterns]; got != test.wantExcludes {
						t.Errorf("local exclude patterns = %q, want %q", got, test.wantExcludes)
					}
				}
				for _, action := range op.GetFile().GetActions() {
					if f := action.GetMkfile(); f != nil && string(f.Data) == "from component" {
						generated = true
					}
				}
			}
			if local != test.wantLocal || !generated {
				t.Errorf("local source = %t, want %t; generated input present = %t", local, test.wantLocal, generated)
			}
		})
	}
}
