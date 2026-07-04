package lower

import (
	"strings"
	"testing"
)

// TestReduceNoInitEmits pins that a reduce with only a callback lowers to the
// value.Array ReduceNoInit method, a plain method call with no initial value.
func TestReduceNoInitEmits(t *testing.T) {
	const src = "export function sum(a: number[]): number { return a.reduce((acc, n) => acc + n); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ReduceNoInit(func(acc float64, n float64) float64 {") {
		t.Errorf("reduce with no initial value did not lower to ReduceNoInit:\n%s", source)
	}
}

// TestReduceWithInitStillEmitsReduce pins that the two-argument form is untouched
// by the no-init dispatch and still lowers to the value.Reduce free function.
func TestReduceWithInitStillEmitsReduce(t *testing.T) {
	const src = "export function sum(a: number[]): number { return a.reduce((acc, n) => acc + n, 0); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Reduce[") {
		t.Errorf("reduce with an initial value did not lower to value.Reduce:\n%s", source)
	}
}
