package build

import (
	"strings"
	"testing"
)

// TestDynamicCalleeCallLowers pins that a call through a dynamically typed callee
// lowers rather than handing back: fn is a parameter typed any, so its shape is
// known only at runtime, and the call dispatches through the boxed value's Call
// method with the argument boxed, the mirror of the dynamic member read.
func TestDynamicCalleeCallLowers(t *testing.T) {
	src := "function apply(fn: any, x: number){ return fn(x); }\nconsole.log(apply((y: number) => y * 2, 21));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("dynamic callee call should lower, got: %v", err)
	}
	if !strings.Contains(out, "fn.Call(") {
		t.Fatalf("expected the dynamic call to dispatch through value.Call, got:\n%s", out)
	}
}

// TestFuncValueBoxesIntoDynamic pins that a static function value flowing into a
// dynamic parameter boxes into a callable value.Value through value.NewFunc, so
// the receiving any slot holds a function a dynamic call site can invoke. The
// wrapper coerces its boxed argument to the declared parameter type and boxes the
// result back.
func TestFuncValueBoxesIntoDynamic(t *testing.T) {
	src := "function apply(fn: any, x: number){ return fn(x); }\nconsole.log(apply((y: number) => y * 2, 21));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("func value into a dynamic slot should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.NewFunc(") {
		t.Fatalf("expected the boxed function to wrap in value.NewFunc, got:\n%s", out)
	}
	if !strings.Contains(out, "value.Arg(") {
		t.Fatalf("expected the wrapper to read its arguments through value.Arg, got:\n%s", out)
	}
}

// TestVoidFuncBoxesIntoDynamic pins that a function with no value result boxes into
// a wrapper that runs the call for its effect and yields undefined, the value a
// call to a void function evaluates to, so a dynamic callback used only for its
// side effect still lowers.
func TestVoidFuncBoxesIntoDynamic(t *testing.T) {
	src := "function each(fn: any){ fn(1); fn(2); }\neach((n: number) => { console.log(n); });\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("void func into a dynamic slot should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.NewFunc(") {
		t.Fatalf("expected the void callback to wrap in value.NewFunc, got:\n%s", out)
	}
	if !strings.Contains(out, "value.Undefined") {
		t.Fatalf("expected the void wrapper to yield value.Undefined, got:\n%s", out)
	}
}

// TestStaticFunctionValueCallUnchanged pins that guarding the dynamic-callee path
// did not disturb a statically typed function value: a callback with a declared
// signature still lowers to a direct Go call on the func value, not through the
// runtime Call, so the fast static path is untouched.
func TestStaticFunctionValueCallUnchanged(t *testing.T) {
	src := "const g: (n: number) => number = y => y * 2;\nconsole.log(g(21));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("static function value call should lower, got: %v", err)
	}
	if strings.Contains(out, ".Call(") {
		t.Fatalf("expected the static call to stay a direct Go call, got:\n%s", out)
	}
}

// TestBoxOptionalParamFuncHandsBack pins that boxing a function with an optional
// parameter into a dynamic value hands back rather than emitting a wrapper that
// would call with the wrong argument count: the call-site defaulting an optional
// parameter needs is a later slice, so the whole unit defers to the interpreter
// instead of miscompiling.
func TestBoxOptionalParamFuncHandsBack(t *testing.T) {
	src := "function apply(fn: any){ return fn(1); }\nconsole.log(apply((x?: number) => (x ?? 0) + 1));\n"
	if _, err := compileSource(t, src); err == nil {
		t.Fatalf("boxing a func with an optional parameter should hand back, but it lowered")
	}
}
