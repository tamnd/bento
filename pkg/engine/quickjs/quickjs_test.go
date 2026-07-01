package quickjs

import (
	"errors"
	"testing"

	"github.com/tamnd/bento/pkg/engine"
)

func newTestEngine(t *testing.T) engine.Engine {
	t.Helper()
	eng, err := engine.New("quickjs")
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

func TestRegisteredAsDefault(t *testing.T) {
	if engine.Default() != "quickjs" {
		t.Errorf("quickjs should be the default backend, got %q", engine.Default())
	}
	found := false
	for _, n := range engine.Available() {
		if n == "quickjs" {
			found = true
		}
	}
	if !found {
		t.Error("quickjs should be listed in Available()")
	}
}

func TestEvalReturnsValue(t *testing.T) {
	eng := newTestEngine(t)
	v, err := eng.Eval("t", `1 + 2 * 3`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got := toFloat(v); got != 7 {
		t.Errorf("want 7, got %v", v)
	}
}

func TestES2023Features(t *testing.T) {
	eng := newTestEngine(t)
	v, err := eng.Eval("t", `
		class C { #x = 5; get x() { return this.#x } }
		const c = new C();
		[1,2,3].at(-1) + c.x + (globalThis.z ?? 0)
	`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got := toFloat(v); got != 8 {
		t.Errorf("want 8, got %v", v)
	}
}

func TestHostFuncRoundTrip(t *testing.T) {
	eng := newTestEngine(t)
	var got []any
	if err := eng.Register("__collect", func(args []any) (any, error) {
		got = args
		return "ok", nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	v, err := eng.Eval("t", `__collect(1, "two", true)`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if v != "ok" {
		t.Errorf("want return \"ok\", got %v", v)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 args, got %v", got)
	}
	if toFloat(got[0]) != 1 || got[1] != "two" || got[2] != true {
		t.Errorf("args round-tripped wrong: %v", got)
	}
}

func TestMicrotaskDrain(t *testing.T) {
	eng := newTestEngine(t)
	var seen any
	if err := eng.Register("__set", func(args []any) (any, error) {
		seen = args[0]
		return nil, nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := eng.Eval("t", `Promise.resolve(41).then(v => __set(v + 1))`); err != nil {
		t.Fatalf("eval: %v", err)
	}
	if seen != nil {
		t.Error("microtask should not have run before drain")
	}
	n, err := eng.DrainMicrotasks()
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n == 0 {
		t.Error("expected at least one job drained")
	}
	if toFloat(seen) != 42 {
		t.Errorf("want 42 after drain, got %v", seen)
	}
}

func TestUncaughtIsException(t *testing.T) {
	eng := newTestEngine(t)
	_, err := eng.Eval("boom.ts", `throw new Error("nope")`)
	if err == nil {
		t.Fatal("expected an error")
	}
	var ex *engine.Exception
	if !errors.As(err, &ex) {
		t.Fatalf("want *engine.Exception, got %T: %v", err, err)
	}
	if ex.Display() == "" {
		t.Error("exception should render a non-empty display string")
	}
}

func TestCallGlobal(t *testing.T) {
	eng := newTestEngine(t)
	if _, err := eng.Eval("t", `globalThis.add = (a, b) => a + b`); err != nil {
		t.Fatalf("eval: %v", err)
	}
	v, err := eng.Call("add", 3, 4)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if toFloat(v) != 7 {
		t.Errorf("want 7, got %v", v)
	}
}

// toFloat coerces a bridged numeric value for assertions.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return -1
	}
}
