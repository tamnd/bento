package lower

import (
	"strings"
	"testing"
)

// node:util is the first module whose exports take values of any type rather than
// strings, so its lowering boxes each argument where the fs and path lowerings check
// each one against a shape. These pin the three members it carries, the arity
// formatWithOptions cannot spell, and the shapes that hand back.

// TestUtilFormatLowersToTheFormatHelper covers the member a program reaches util for
// most, in both the named and the module-object import form, since a specifier string
// with a boxed argument after it is the whole point of the module.
func TestUtilFormatLowersToTheFormatHelper(t *testing.T) {
	forms := []struct {
		name string
		src  string
	}{
		{"named", `import { format } from "node:util";
const s: string = format("%s is %d", "x", 2);`},
		{"default", `import util from "node:util";
const s: string = util.format("%s is %d", "x", 2);`},
		{"namespace", `import * as util from "node:util";
const s: string = util.format("%s is %d", "x", 2);`},
	}
	want := `value.NodeFormat(value.StringValue(value.FromGoString("%s is %d")), value.StringValue(value.FromGoString("x")), value.Number(2))`
	for _, f := range forms {
		t.Run(f.name, func(t *testing.T) {
			if got := renderProgram(t, f.src); !strings.Contains(got, want) {
				t.Errorf("%s import did not emit %s:\n%s", f.name, want, got)
			}
		})
	}
}

// TestUtilInspectLowersWithItsOptions pins that inspect's second argument rides along
// as a boxed value rather than being read at compile time. The options are a runtime
// object the helper reads, so an options object built at runtime works the same as a
// literal one.
func TestUtilInspectLowersWithItsOptions(t *testing.T) {
	got := renderProgram(t, `import { inspect } from "node:util";
const s: string = inspect({ a: 1 }, { depth: 0 });`)
	want := `value.NodeInspectArgs(value.NewObject().Set(value.FromGoString("a"), value.Number(1)), value.NewObject().Set(value.FromGoString("depth"), value.Number(0)))`
	if !strings.Contains(got, want) {
		t.Errorf("did not emit %s:\n%s", want, got)
	}
}

// TestUtilFormatWithOptionsPassesItsOptionsFirst pins the shape of the helper call:
// the options object is a required parameter and the format arguments are the
// variadic rest, so the emitted call is the source order unchanged.
func TestUtilFormatWithOptionsPassesItsOptionsFirst(t *testing.T) {
	got := renderProgram(t, `import { formatWithOptions } from "node:util";
const s: string = formatWithOptions({ numericSeparator: true }, "%d", 1234567);`)
	want := `value.NodeFormatWithOptions(value.NewObject().Set(value.FromGoString("numericSeparator"), value.Bool(true)), value.StringValue(value.FromGoString("%d")), value.Number(1234567))`
	if !strings.Contains(got, want) {
		t.Errorf("did not emit %s:\n%s", want, got)
	}
}

// TestUtilFormatWithOptionsWithNoArgumentsStillCompiles is the one arity the helper's
// Go signature cannot take as written, since its first parameter is required. Node
// throws ERR_INVALID_ARG_TYPE for the call, so the lowering emits undefined as the
// options and lets the helper throw it, rather than handing the unit back over a
// call whose only possible outcome is that error.
//
// The call is a type error too, since the ambient declaration types the options as
// required, so it renders through the tolerant path: the AOT front door admits an
// argument-count mismatch (2554) because the call still lowers, which is exactly what
// this pins.
func TestUtilFormatWithOptionsWithNoArgumentsStillCompiles(t *testing.T) {
	got := renderProgramTolerant(t, `import { formatWithOptions } from "node:util";
const s: string = formatWithOptions();`)
	if !strings.Contains(got, "value.NodeFormatWithOptions(value.Undefined)") {
		t.Errorf("did not emit value.NodeFormatWithOptions(value.Undefined):\n%s", got)
	}
}

// TestUtilFormatOfNoArgumentsIsTheEmptyString covers format's own zero-argument call,
// which is legal in Node and answers "". The helper is fully variadic, so nothing
// special is emitted, and this pins that the lowering does not invent an argument.
func TestUtilFormatOfNoArgumentsIsTheEmptyString(t *testing.T) {
	got := renderProgram(t, `import { format } from "node:util";
const s: string = format();`)
	if !strings.Contains(got, "value.NodeFormat()") {
		t.Errorf("did not emit value.NodeFormat():\n%s", got)
	}
}

// TestUtilHandbacks pins what is not there yet. A member util does not carry must
// name itself in the reason, since the module now exists and a reader would otherwise
// be told the whole import was the problem.
func TestUtilHandbacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"a member the module does not carry",
			`import util from "node:util";
const f: any = util.promisify(null);`,
			"call of promisify on node:util is a later slice",
		},
		{
			"an export read as a value rather than called",
			`import util from "node:util";
const f: any = util.format;`,
			"reading format off node:util as a value is a later slice",
		},
		{
			"an argument that does not box yet",
			`import { format } from "node:util";
const m = new Map<string, number>();
const s: string = format("%s", m);`,
			"util.format with an argument that does not box yet",
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
