package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestUnmodeledGlobalObjectReadHandsBack pins that reading an ambient global
// constructor as an object (RegExp.length, String.length) hands back rather than
// lowering. bento has no generated Go behind the RegExp or String name, so the
// member's object operand used to lower to a bare capitalized identifier and the
// generated Go named an undefined symbol like RegExp, failing to build. Handing
// back routes the unit to the interpreter until the global's object form is
// modeled. RegExp/S15.10.5_A1 and String/S15.5.3_A1 hit exactly this.
func TestUnmodeledGlobalObjectReadHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"regexp-length", "const n = RegExp.length;\nconsole.log(String(n));\n"},
		{"string-length", "const n = String.length;\nconsole.log(String(n));\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := compile(t, c.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, "ambient global") {
				t.Errorf("hand-back reason = %q, want it to mention the ambient global", nyl.Reason)
			}
		})
	}
}

// TestAmbientGlobalAssignmentHandsBack pins that assigning to an ambient global
// (NaN = 12) hands back rather than emitting a store into an undefined Go symbol.
// The runtime holds no lvalue for the global, and the strict-mode TypeError the
// store throws is not modeled, so handing back keeps the unit truthful.
// global/10.2.1.1.3-4-16-s hits this.
func TestAmbientGlobalAssignmentHandsBack(t *testing.T) {
	const src = "NaN = 12;\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "ambient global") {
		t.Errorf("hand-back reason = %q, want it to mention the ambient global", nyl.Reason)
	}
}

// TestModeledGlobalStillLowers pins that the modeled ambient globals keep their
// dedicated lowering and are not swallowed by the unmodeled-global handback: NaN
// and Infinity read as the doubles they name, and a Math member call still routes
// to the Go math package. The guard sits after these paths return, so it catches
// only the unmodeled remainder.
func TestModeledGlobalStillLowers(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"nan", "export function k(): number { return NaN; }\n", "math.NaN()"},
		{"infinity", "export function k(): number { return Infinity; }\n", "math.Inf("},
		{"math-floor", "export function k(x: number): number { return Math.floor(x); }\n", "math.Floor("},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := renderProgram(t, c.src)
			if !strings.Contains(out, c.want) {
				t.Fatalf("modeled global %s did not lower to %q:\n%s", c.name, c.want, out)
			}
		})
	}
}
