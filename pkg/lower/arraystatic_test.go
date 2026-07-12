package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestArrayStaticEmits pins the shape: Array.of builds the same value.NewArray
// an array literal does, instantiated at the checker's element type, so the
// arguments become the elements one to one.
func TestArrayStaticEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"of",
			"export function trio(): number[] { return Array.of(1, 2, 3); }\n",
			"value.NewArray[float64](1, 2, 3)",
		},
		{
			"ofStrings",
			"export function names(): string[] { return Array.of(\"a\", \"b\"); }\n",
			"value.NewArray[value.BStr](value.FromGoString(\"a\"), value.FromGoString(\"b\"))",
		},
		{
			"fromArray",
			"export function copy(a: number[]): number[] { return Array.from(a); }\n",
			"value.ArrayFrom(append([]float64{}, a.Elems()...))",
		},
		{
			"fromString",
			"export function chars(s: string): string[] { return Array.from(s); }\n",
			"value.ArrayFrom(s.CodePoints())",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("Array static did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestArrayStaticHandsBack pins the boundary: Array.from with a thisArg, the
// third argument that binds this inside the map callback, is its own later slice,
// so it hands back with a named reason rather than falling through to the
// receiver-typed method dispatch.
func TestArrayStaticHandsBack(t *testing.T) {
	const src = "export function twice(a: number[]): number[] { return Array.from(a, (x: number) => x, null); }\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "Array.from with a thisArg") {
		t.Errorf("hand-back reason = %q, want it to mention Array.from with a thisArg", nyl.Reason)
	}
}

// TestArrayIsArrayEmits pins the brand check: a dynamic value dispatches through
// value.IsArray, a statically typed array folds to true, and any other static
// type folds to false.
func TestArrayIsArrayEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"dynamic",
			"export function f(x: any): boolean { return Array.isArray(x); }\n",
			"value.IsArray(x)",
		},
		{
			"typedArray",
			"export function f(a: number[]): boolean { return Array.isArray(a); }\n",
			"return true",
		},
		{
			"nonArray",
			"export function f(s: string): boolean { return Array.isArray(s); }\n",
			"return false",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("Array.isArray did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestArrayFromDynamicEmits pins the dynamic form: Array.from over a boxed source
// lowers to value.ArrayFromArrayLike, which reads the source's length and integer
// keys at runtime, with value.Undefined standing in for an absent map callback.
func TestArrayFromDynamicEmits(t *testing.T) {
	const src = "export function f(o: any): any { return Array.from(o); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ArrayFromArrayLike(o, value.Undefined)") {
		t.Errorf("Array.from over a dynamic source did not lower to ArrayFromArrayLike:\n%s", source)
	}
}
