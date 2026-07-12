package lower

import (
	"strings"
	"testing"
)

// TestIterHelperEmits pins the shape: map and filter over an array iterator lower to
// the value.Iter* free function taking the receiver's Next, the two chain by nesting
// each helper's Next inside the one above it, a manual next() on a helper drives it
// directly, and a for...of over a helper pulls it with a no-argument Next until done.
func TestIterHelperEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"map",
			"export function f(a: number[]): void { a.values().map((n: number): number => n * 2); }\n",
			"value.IterMap(",
		},
		{
			"filter",
			"export function f(a: number[]): void { a.values().filter((n: number): boolean => n > 0); }\n",
			"value.IterFilter(",
		},
		{
			"chain",
			"export function f(a: number[]): void { a.values().map((n: number): number => n + 1).filter((n: number): boolean => n > 0); }\n",
			"value.IterFilter(value.IterMap(",
		},
		{
			"next",
			"export function f(a: number[]): number { const it = a.values().map((n: number): number => n * 2); const r = it.next(); return r.done ? 0 : 1; }\n",
			"it.Next()",
		},
		{
			"forof",
			"export function f(a: number[]): number { let s = 0; for (const v of a.values().map((n: number): number => n * 2)) { s += v; } return s; }\n",
			".Next()",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("iterator helper did not print %q:\n%s", tc.want, source)
			}
		})
	}
}
