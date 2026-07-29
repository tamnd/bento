package lower

import (
	"strings"
	"testing"
)

// node:os is the module a program asks about the machine it is on. Its functions
// all take nothing and answer one string or one number, so they lower through a
// single path, and these cases pin that path at both ends: every export reaches
// its helper through every import form, and the set of exports the lowerer admits
// is exactly the set it can emit.

// TestOSFunctionsLowerThroughEveryImportForm calls each export as a member of a
// module-object binding and as the bare name a named import bound, and requires
// both to reach the same helper. Node's core modules are CommonJS, so os.arch()
// and the arch() of import { arch } name one function; a program that picked one
// import form has not picked a different implementation.
func TestOSFunctionsLowerThroughEveryImportForm(t *testing.T) {
	for name, helper := range osHelpers {
		t.Run(name, func(t *testing.T) {
			want := "value." + helper + "()"
			forms := map[string]string{
				"named":     `import { ` + name + ` } from "node:os";` + "\nconst v: any = " + name + "();",
				"default":   `import os from "node:os";` + "\nconst v: any = os." + name + "();",
				"namespace": `import * as os from "node:os";` + "\nconst v: any = os." + name + "();",
			}
			for form, src := range forms {
				got := renderProgram(t, src)
				if !strings.Contains(got, want) {
					t.Errorf("%s import of %s emitted:\n%s\nwant a call of %s", form, name, got, want)
				}
			}
		})
	}
}

// TestOSExportSetIsExactlyWhatLowers pins the three lists that describe node:os
// against each other. nodeModuleExports decides which imports are admitted rather
// than handed back, and the helper maps decide what each admitted name emits, so a
// name admitted with no helper is the one combination that compiles to a call of a
// function the value package does not declare. That is a build that fails at the
// Go compiler with the reason lost, which is the shape this slice's whole design
// is arranged to avoid.
func TestOSExportSetIsExactlyWhatLowers(t *testing.T) {
	for name := range nodeModuleExports["node:os"] {
		_, isFunc := osHelpers[name]
		_, isConst := nodeModuleConstants["node:os"][name]
		if !isFunc && !isConst {
			t.Errorf("node:os admits an import of %s but has nothing to lower it to", name)
		}
		if isFunc && isConst {
			t.Errorf("node:os lowers %s both as a call and as a value read", name)
		}
	}
	for name := range osHelpers {
		if !nodeModuleExports["node:os"][name] {
			t.Errorf("node:os lowers %s but hands back an import of it", name)
		}
	}
	for name := range nodeModuleConstants["node:os"] {
		if !nodeModuleExports["node:os"][name] {
			t.Errorf("node:os lowers %s but hands back an import of it", name)
		}
	}
}

// TestOSObjectExportsAreNotClaimed pins the part of node:os that is still the
// engine's. os.cpus and its kin answer with an object or an array, which the
// compiled path has no way to build yet, so the lowerer must not claim them: an
// import of one hands back and routes the unit to the engine, which answers it.
//
// This is checked against the export set rather than by lowering a fixture. A
// fixture cannot reach the handback here, because the ambient declarations these
// tests compile against name only what bento lowers, so the checker rejects the
// import before the lowerer sees it. A real program with @types/node installed
// type-checks and does reach it, which is the path this set guards.
func TestOSObjectExportsAreNotClaimed(t *testing.T) {
	for _, name := range []string{"cpus", "networkInterfaces", "userInfo", "loadavg", "constants"} {
		if nodeModuleExports["node:os"][name] {
			t.Errorf("node:os claims to lower %s, which answers with an object", name)
		}
	}
}

// TestOSFunctionsWithArgumentsHandBack pins the guard on the one path every os
// export shares. None of them takes an argument, so a call with one is not the
// call the helper implements, and lowering it would silently drop what the program
// passed.
func TestOSFunctionsWithArgumentsHandBack(t *testing.T) {
	src := `import * as os from "node:os";
const v: any = os.arch("x64");`
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatal("lowered, want a hand back")
	} else if want := "os.arch takes no arguments"; !strings.Contains(err.Error(), want) {
		t.Errorf("hand back said %q, want it to mention %q", err.Error(), want)
	}
}
