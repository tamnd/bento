package lower

import (
	"strings"
	"testing"
)

// TestFindLastEmits pins that a.findLast(fn) lowers to the value.Array FindLast
// method over the lowered arrow, the same callback shape find uses.
func TestFindLastEmits(t *testing.T) {
	const src = "export function last(a: number[]): number | undefined { return a.findLast((n) => n > 2); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".FindLast(func(n float64) bool {") {
		t.Errorf("findLast did not lower to the Array method:\n%s", source)
	}
}

// TestFindLastIndexEmits pins that a.findLastIndex(fn) lowers to FindLastIndex.
func TestFindLastIndexEmits(t *testing.T) {
	const src = "export function lasti(a: number[]): number { return a.findLastIndex((n) => n > 2); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".FindLastIndex(func(n float64) bool {") {
		t.Errorf("findLastIndex did not lower to the Array method:\n%s", source)
	}
}
