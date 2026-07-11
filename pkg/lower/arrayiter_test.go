package lower

import (
	"strings"
	"testing"
)

// TestArrayIterEmits pins the shape: values, keys, and entries each mint a
// value.ArrayIterFromTyped over the receiver with the kind constant they name, and
// a manual it.next() lowers to the runtime's Next.
func TestArrayIterEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"values",
			"export function f(a: number[]): void { a.values(); }\n",
			"value.ArrayIterFromTyped(a, value.ArrayIterValues,",
		},
		{
			"keys",
			"export function f(a: number[]): void { a.keys(); }\n",
			"value.ArrayIterFromTyped(a, value.ArrayIterKeys,",
		},
		{
			"entries",
			"export function f(a: string[]): void { a.entries(); }\n",
			"value.ArrayIterFromTyped(a, value.ArrayIterEntries,",
		},
		{
			"next",
			"export function f(a: number[]): number { const it = a.values(); const r = it.next(); return r.done ? 0 : 1; }\n",
			"it.Next()",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("array iterator did not print %q:\n%s", tc.want, source)
			}
		})
	}
}
