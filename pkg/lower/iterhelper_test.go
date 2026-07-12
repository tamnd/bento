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
			"flatmap",
			"export function f(a: number[]): void { a.values().flatMap((n: any): any => n); }\n",
			"value.IterFlatMap(",
		},
		{
			"take",
			"export function f(a: number[]): void { a.values().take(3); }\n",
			"value.IterTake(",
		},
		{
			"drop",
			"export function f(a: number[]): void { a.values().drop(3); }\n",
			"value.IterDrop(",
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
		{
			"reduce",
			"export function f(a: number[]): void { console.log(a.values().reduce((acc: number, n: number): number => acc + n, 0)); }\n",
			"value.IterReduce(",
		},
		{
			"reduce_no_init",
			"export function f(a: number[]): void { console.log(a.values().reduce((acc: number, n: number): number => acc + n)); }\n",
			"value.Undefined",
		},
		{
			"toarray",
			"export function f(a: number[]): void { console.log(a.values().map((n: number): number => n * 2).toArray()); }\n",
			"value.IterToArray(",
		},
		{
			"foreach",
			"export function f(a: number[]): void { a.values().forEach((n: number): void => { console.log(n); }); }\n",
			"value.IterForEach(",
		},
		{
			"some",
			"export function f(a: number[]): boolean { return a.values().some((n: number): boolean => n > 0); }\n",
			"value.IterSome(",
		},
		{
			"every",
			"export function f(a: number[]): boolean { return a.values().every((n: number): boolean => n > 0); }\n",
			"value.IterEvery(",
		},
		{
			"find",
			"export function f(a: number[]): void { console.log(a.values().find((n: number): boolean => n > 2)); }\n",
			"value.IterFind(",
		},
		{
			"from_array",
			"export function f(): void { Iterator.from([1, 2, 3]).map((n: number): number => n * 2).toArray(); }\n",
			"value.IterFrom(",
		},
		{
			"from_string",
			"export function f(): void { Iterator.from(\"abc\").toArray(); }\n",
			"value.IterFrom(",
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
