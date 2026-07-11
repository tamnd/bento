package value

import (
	"testing"
)

// TestAsyncGenYieldAndAwait drives an async generator whose body yields, awaits between
// yields, and completes, checking each pull settles the way the async iterator protocol
// says: a yield fulfills the pull with { value, done: false }, a completion fulfills it
// with a done result, and the await between the two yields keeps the second pull pending
// until the awaited promise settles on the microtask queue. Each pull's promise is read
// after the queue drains, the settle a for await...of observes when it awaits the pull.
func TestAsyncGenYieldAndAwait(t *testing.T) {
	// yield 1; await Promise.resolve(0); yield 2; (completes with undefined)
	g := NewAsyncGen(func(co *AsyncGenCo[float64]) Value {
		co.Yield(1)
		AsyncGenAwait(co, Resolved(0.0))
		co.Yield(2)
		return Undefined
	})
	box := func(y float64) Value { return Number(y) }

	p1 := g.Next(Undefined, box)
	RunMicrotasks()
	p2 := g.Next(Undefined, box)
	RunMicrotasks()
	p3 := g.Next(Undefined, box)
	RunMicrotasks()

	if got := (result{ToNumber(p1.value.Value), p1.value.Done}); got != (result{1, false}) {
		t.Fatalf("pull 1 = %+v, want {1 false}", got)
	}
	if got := (result{ToNumber(p2.value.Value), p2.value.Done}); got != (result{2, false}) {
		t.Fatalf("pull 2 = %+v, want {2 false}", got)
	}
	if !p3.value.Done {
		t.Fatalf("pull 3 done = false, want true")
	}
}

// result is the { number, done } pair the test compares each pull's settled IterResult
// against, so a mismatch prints both fields together.
type result struct {
	n    float64
	done bool
}
