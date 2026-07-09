package build

import (
	"strings"
	"testing"
	"time"
)

// TestModuleThisHandsBackBounded pins that binding a module-scope `this`, which
// the checker resolves to the whole global object, hands back in bounded time
// rather than exhausting memory. The global object type is a tree of hundreds of
// properties whose types are more objects still, and the checker hands a fresh id
// at each step, so the structural-key walk that interns object shapes used to
// recurse without bound and climb past several GB in seconds, OOM-killing the
// process or the machine. The node budget on the walk now stops it and the type
// hands back, the worst case the lowerer is allowed to produce.
func TestModuleThisHandsBackBounded(t *testing.T) {
	src := "var global = this;\nconsole.log(typeof global);\n"
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := compileSource(t, src)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		if r.err == nil {
			t.Fatalf("binding module-scope this should hand back, got Go:\n%s", r.out)
		}
		if !strings.Contains(r.err.Error(), "too large to key structurally") {
			t.Fatalf("unexpected error, want the structural-key handback, got: %v", r.err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("binding module-scope this did not finish in 30s: the structural-key walk is running memory away")
	}
}
