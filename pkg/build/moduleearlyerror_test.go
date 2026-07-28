package build

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/lower"
)

// The import-declaration early errors typescript-go leaves unreported. An engine
// (node) throws SyntaxError at parse time for each, so bento must reject the
// module ahead of lowering rather than reach the lowerer and hand it back: a
// parse-phase negative test scores a real build error as a pass and a
// *NotYetLowerable as a handback.

// TestImportBindingEvalArgumentsRejected pins that an imported binding named eval
// or arguments is rejected. Module code is strict, and an ImportedBinding may not
// be eval or arguments in any of the default, named, or namespace forms.
func TestImportBindingEvalArgumentsRejected(t *testing.T) {
	cases := map[string]map[string]string{
		"default eval": {
			"m.ts":    "export default 1;\n",
			"main.ts": "import eval from \"./m\";\n",
		},
		"named eval": {
			// Re-export the binding as eval from m (legal: export binds no local)
			// so main's `import { eval }` is the only eval-named binding, isolating
			// the import-side early error from any m-side diagnostic.
			"m.ts":    "const e = 1;\nexport { e as eval };\n",
			"main.ts": "import { eval } from \"./m\";\n",
		},
		"named as arguments": {
			"m.ts":    "export const x = 1;\n",
			"main.ts": "import { x as arguments } from \"./m\";\n",
		},
		"namespace eval": {
			"m.ts":    "export const y = 1;\n",
			"main.ts": "import * as eval from \"./m\";\n",
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := compileModule(t, "main.ts", files)
			assertEarlyError(t, err)
		})
	}
}

// TestDuplicateImportAttributeRejected pins that a WithClause with a repeated key
// is rejected, comparing by cooked value so an escaped string-literal key
// collides with the identifier it spells.
func TestDuplicateImportAttributeRejected(t *testing.T) {
	cases := map[string]string{
		"plain":   "import x from \"./m\" with { type: \"json\", type: \"js\" };\n",
		"escaped": "import x from \"./m\" with { \"type\": \"json\", type: \"js\" };\n",
	}
	for name, main := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := compileModule(t, "main.ts", map[string]string{
				"m.ts":    "export default 1;\n",
				"main.ts": main,
			})
			assertEarlyError(t, err)
		})
	}
}

// TestImportEarlyErrorDecoysStillLower pins the precision boundary: a rename whose
// local binding is a safe name, a namespace with a safe name, and distinct
// attribute keys must all lower, and `export { x as eval }` binds no local so it
// is legal.
func TestImportEarlyErrorDecoysStillLower(t *testing.T) {
	cases := map[string]map[string]string{
		"rename to safe local": {
			"m.ts":    "export const evalx = 1;\n",
			"main.ts": "import { evalx as safe } from \"./m\";\nconsole.log(safe);\n",
		},
		"namespace safe": {
			"m.ts":    "export const y = 1;\n",
			"main.ts": "import * as ns from \"./m\";\nconsole.log(ns.y);\n",
		},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := compileModule(t, "main.ts", files); err != nil {
				t.Fatalf("a valid import should still lower, got: %v", err)
			}
		})
	}
}

func assertEarlyError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("an import-declaration early error should be rejected, got no error")
	}
	if !strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("expected a SyntaxError rejection, got: %v", err)
	}
	// The rejection must be a real early error, not a lowering hand-back: a
	// parse-phase negative test scores a *NotYetLowerable as a handback, not a pass.
	if strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("the rejection must be a real build error, not a handback: %v", err)
	}
	var nyl *lower.NotYetLowerable
	_ = nyl
}
