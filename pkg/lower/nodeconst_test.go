package lower

import (
	"strings"
	"testing"
)

// A node module's data exports, path.sep and path.delimiter, os.EOL and
// os.devNull, are read rather than called, so they take a different route
// through the lowerer than every export before them. These cases pin both ends
// of that route: the two forms a program reads one in, and the shapes that still
// have no lowering.

// TestModuleConstantsReadThroughEveryImportForm reads each data export as a
// member of a module-object binding and as the bare name a named import bound,
// and requires both to reach the same helper. Node's core modules are CommonJS,
// so the default export, the namespace binding and the named import all name one
// module object; a program that picked one of them has not picked a different
// value for sep.
func TestModuleConstantsReadThroughEveryImportForm(t *testing.T) {
	cases := []struct {
		module string
		name   string
		want   string
	}{
		{"node:path", "sep", "value.PathSep()"},
		{"node:path", "delimiter", "value.PathDelimiter()"},
		{"node:os", "EOL", "value.OSEOL()"},
		{"node:os", "devNull", "value.OSDevNull()"},
	}
	for _, tc := range cases {
		t.Run(tc.module+"."+tc.name, func(t *testing.T) {
			forms := map[string]string{
				"named":     `import { ` + tc.name + ` } from "` + tc.module + `";` + "\nconst v: any = " + tc.name + ";",
				"default":   `import m from "` + tc.module + `";` + "\nconst v: any = m." + tc.name + ";",
				"namespace": `import * as m from "` + tc.module + `";` + "\nconst v: any = m." + tc.name + ";",
			}
			for form, src := range forms {
				got := renderProgram(t, src)
				if !strings.Contains(got, tc.want) {
					t.Errorf("%s import of %s emitted:\n%s\nwant a read of %s", form, tc.name, got, tc.want)
				}
			}
		})
	}
}

// TestModuleConstantHandbacks pins the shapes around the data exports that do not
// lower. Calling one is not a thing a module member can do, and an exported
// function read as a value wants a Go func bento does not have, since every
// export lowers as a call shape rather than as a func of the right signature.
// Before this slice the bare read of an exported function compiled to the local
// binding's name, which nothing in the generated package declares, so the handback
// is what turns a broken build into a route to the engine.
func TestModuleConstantHandbacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"a named import of a function read as a value",
			`import { join } from "node:path";
const v: any = join;`,
			"the node:path export join used as a function value is a later slice",
		},
		{
			"a member function read as a value",
			`import * as path from "node:path";
const v: any = path.join;`,
			"reading join off node:path as a value is a later slice",
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

// TestCallingAModuleConstantHandsBack pins the call dispatch's own guard. A
// program cannot reach it through a source fixture, because calling a string is
// a diagnostic the tolerant compile does not admit, so it is exercised where it
// lives: the dispatch is reached with a data export's name, the pair a member
// call on a module-object binding would hand it, and it has to name the shape
// rather than report the export as an unimplemented function.
func TestCallingAModuleConstantHandsBack(t *testing.T) {
	r := NewRenderer(nil)
	for module, names := range nodeModuleConstants {
		for name := range names {
			if _, err := r.nodeBuiltinCall(nodeBuiltin{module: module, name: name}, nil); err == nil {
				t.Errorf("call of %s.%s lowered, want a hand back", module, name)
			} else if want := module + "." + name + " is a value, not a function"; !strings.Contains(err.Error(), want) {
				t.Errorf("call of %s.%s said %q, want it to mention %q", module, name, err.Error(), want)
			}
		}
	}
}
