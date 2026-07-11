package value

import "testing"

// TestGenYieldsInOrder proves the coroutine yields its values in body order and
// then reports done: a body that yields 1 then 2 pulls 1 not-done, 2 not-done, and
// a third pull done. The body runs in a goroutine that suspends on each yield, so
// the values arrive one pull at a time rather than all at once.
func TestGenYieldsInOrder(t *testing.T) {
	g := NewGen(func(co *GenCo[float64]) Value {
		co.Yield(1)
		co.Yield(2)
		return Undefined
	})
	if v, done := g.Next(Undefined); v != 1 || done {
		t.Fatalf("first pull = (%v, %v), want (1, false)", v, done)
	}
	if v, done := g.Next(Undefined); v != 2 || done {
		t.Fatalf("second pull = (%v, %v), want (2, false)", v, done)
	}
	if v, done := g.Next(Undefined); v != 0 || !done {
		t.Fatalf("third pull = (%v, %v), want (0, true)", v, done)
	}
}

// TestGenLazyBody proves the body does not run until the first pull: a body that
// records a side effect on entry leaves it unset until Next is called, matching the
// JavaScript rule that calling a generator function only creates the object.
func TestGenLazyBody(t *testing.T) {
	ran := false
	g := NewGen(func(co *GenCo[float64]) Value {
		ran = true
		co.Yield(1)
		return Undefined
	})
	if ran {
		t.Fatal("body ran before the first pull")
	}
	g.Next(Undefined)
	if !ran {
		t.Fatal("body did not run on the first pull")
	}
}

// TestGenSentValue proves next(v) threads its argument back as the value the yield
// evaluates to: the body binds each yield's result and yields it back doubled, so
// pulling with 10 makes the second yield report 20.
func TestGenSentValue(t *testing.T) {
	g := NewGen(func(co *GenCo[float64]) Value {
		x := co.Yield(0)
		co.Yield(ToNumber(x) * 2)
		return Undefined
	})
	g.Next(Undefined)
	if v, _ := g.Next(Number(10)); v != 20 {
		t.Fatalf("sent-value pull = %v, want 20", v)
	}
}

// TestGenReturnValue proves the body's return value is carried on completion: a
// body that returns 7 latches done with Result 7, the value a { value, done: true }
// result reports.
func TestGenReturnValue(t *testing.T) {
	g := NewGen(func(co *GenCo[float64]) Value {
		co.Yield(1)
		return Number(7)
	})
	g.Next(Undefined)
	if v, done := g.Next(Undefined); v != 0 || !done {
		t.Fatalf("completion pull = (%v, %v), want (0, true)", v, done)
	}
	if got := g.Result(); ToNumber(got) != 7 {
		t.Fatalf("Result = %v, want 7", ToNumber(got))
	}
}

// TestGenReturnClosesEarly proves Return unwinds a suspended body and completes it
// with the given value: a generator paused at its first yield reports done after
// Return, and a further pull stays done.
func TestGenReturnClosesEarly(t *testing.T) {
	g := NewGen(func(co *GenCo[float64]) Value {
		co.Yield(1)
		co.Yield(2)
		return Undefined
	})
	g.Next(Undefined)
	if v, done := g.Return(Number(99)); v != 0 || !done {
		t.Fatalf("Return = (%v, %v), want (0, true)", v, done)
	}
	if ToNumber(g.Result()) != 99 {
		t.Fatalf("Result after Return = %v, want 99", ToNumber(g.Result()))
	}
	if _, done := g.Next(Undefined); !done {
		t.Fatal("pull after Return is not done")
	}
}
