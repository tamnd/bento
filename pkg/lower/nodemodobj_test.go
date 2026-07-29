package lower

import (
	"strings"
	"testing"
)

// A node: core module is normally reached through the whole module object rather
// than through named imports: `import path from "node:path"` and `import * as path
// from "node:path"` are how Node programs are written, and until this slice both
// handed the unit back to the engine while `import { join }` compiled. These tests
// pin that all three forms reach the same builtin, and that the shapes that still
// have no lowering hand back naming what they were.

// TestModuleObjectImportsLowerLikeNamed proves the three import forms of one
// export emit the same call.
func TestModuleObjectImportsLowerLikeNamed(t *testing.T) {
	forms := []struct {
		name string
		src  string
	}{
		{"named", `import { join } from "node:path";
const p: string = join("a", "b");`},
		{"default", `import path from "node:path";
const p: string = path.join("a", "b");`},
		{"namespace", `import * as path from "node:path";
const p: string = path.join("a", "b");`},
	}
	var first string
	for i, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			got := renderProgram(t, f.src)
			if !strings.Contains(got, `value.PathJoin(value.FromGoString("a"), value.FromGoString("b"))`) {
				t.Fatalf("%s import did not lower to PathJoin:\n%s", f.name, got)
			}
			if i == 0 {
				first = got
				return
			}
			if got != first {
				t.Errorf("%s import emitted different Go than the named import:\n--- %s ---\n%s\n--- named ---\n%s", f.name, f.name, got, first)
			}
		})
	}
}

// TestMixedDefaultAndNamedImport covers the clause that carries both, where the
// default binding must be recorded without swallowing the named list.
func TestMixedDefaultAndNamedImport(t *testing.T) {
	got := renderProgram(t, `import path, { join } from "node:path";
const a: string = path.join("a", "b");
const b: string = join("c", "d");`)
	for _, want := range []string{`value.PathJoin(value.FromGoString("a"), value.FromGoString("b"))`, `value.PathJoin(value.FromGoString("c"), value.FromGoString("d"))`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

// TestModuleObjectImportOtherModules proves the form is not special-cased to
// node:path: fs and os reach their builtins the same way.
func TestModuleObjectImportOtherModules(t *testing.T) {
	got := renderProgram(t, `import os from "node:os";
import * as fs from "node:fs";
const dir: string = os.tmpdir();
fs.writeFileSync(dir, "x");`)
	for _, want := range []string{"value.Tmpdir()", "value.WriteFileSync("} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

// TestModuleObjectHandbacks pins the shapes that still have no lowering. Each
// must hand back naming the module, since a reason that blames a missing receiver
// or an undeclared local sends a reader looking in the wrong place.
func TestModuleObjectHandbacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"a member the module does not export",
			`import path from "node:path";
const p: any = path.parse("a");`,
			"call of parse on node:path is a later slice",
		},
		{
			"an export read as a value rather than called",
			`import path from "node:path";
const j: any = path.join;`,
			"reading join off node:path as a value is a later slice",
		},
		{
			"the module used as a whole value",
			`import path from "node:path";
const m: any = path;`,
			"the node:path module used as a whole value is a later slice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compileTolerant(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			if err == nil {
				t.Fatal("lowered, want a hand back")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("hand back said %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}
