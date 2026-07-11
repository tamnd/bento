package value

import "testing"

// TestAwaitDefersContinuation proves an async body runs synchronously up to its first
// await, parks there, and resumes only at the microtask checkpoint, so the code after
// an await runs in a later turn than the call that started the body.
func TestAwaitDefersContinuation(t *testing.T) {
	drainReset()
	var order []string
	order = append(order, "before")
	p := RunAsync(func(co *AsyncCo) float64 {
		order = append(order, "body-start")
		v := Await(co, Resolved(10.0))
		order = append(order, "after-await")
		return v
	})
	order = append(order, "after-call")
	RunMicrotasks()
	order = append(order, "drained")

	want := []string{"before", "body-start", "after-call", "after-await", "drained"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
	if p.state != promiseFulfilled || p.value != 10 {
		t.Errorf("promise did not fulfill with the awaited value: state=%d value=%v", p.state, p.value)
	}
}

// TestAwaitPendingResumesOnSettle proves a body awaiting a promise that is still
// pending parks until that promise settles, then resumes with its value.
func TestAwaitPendingResumesOnSettle(t *testing.T) {
	drainReset()
	pending := &Promise[float64]{}
	var got float64
	done := false
	RunAsyncVoid(func(co *AsyncCo) {
		got = Await(co, pending)
		done = true
	})
	if done {
		t.Fatal("body ran past the await before the promise settled")
	}
	pending.fulfill(42)
	RunMicrotasks()
	if !done || got != 42 {
		t.Errorf("body did not resume with the settled value: done=%v got=%v", done, got)
	}
}

// TestAwaitRejectedRaisesIntoBody proves awaiting a rejected promise raises the
// rejection at the await, where a try/catch in the body (a Go recover here) catches it.
func TestAwaitRejectedRaisesIntoBody(t *testing.T) {
	drainReset()
	caught := ""
	RunAsyncVoid(func(co *AsyncCo) {
		rej := &Promise[float64]{}
		rej.reject(NewError(FromGoString("boom")))
		func() {
			defer func() {
				if r := recover(); r != nil {
					if e, ok := r.(*Error); ok {
						caught = e.Message().ToGoString()
					}
				}
			}()
			Await(co, rej)
		}()
	})
	RunMicrotasks()
	if caught != "boom" {
		t.Errorf("await on a rejected promise did not raise into the body: caught=%q", caught)
	}
}

// TestRunAsyncRejectsOnThrow proves an async body that throws a value it does not
// catch settles its promise as rejected with that value.
func TestRunAsyncRejectsOnThrow(t *testing.T) {
	drainReset()
	p := RunAsync(func(co *AsyncCo) float64 {
		_ = Await(co, Resolved(1.0))
		panic(NewError(FromGoString("bang")))
	})
	RunMicrotasks()
	if p.state != promiseRejected {
		t.Fatalf("body throw did not reject the promise: state=%d", p.state)
	}
	if e, ok := p.reason.(*Error); !ok || e.Message().ToGoString() != "bang" {
		t.Errorf("rejection reason wrong: %v", p.reason)
	}
}

// TestAwaitValueDefersPlainValue proves awaiting a plain value suspends for one
// microtask turn and then yields the value, the same delay a real await imposes.
func TestAwaitValueDefersPlainValue(t *testing.T) {
	drainReset()
	var order []string
	RunAsyncVoid(func(co *AsyncCo) {
		order = append(order, "start")
		v := AwaitValue(co, 5.0)
		order = append(order, "resumed")
		if v != 5 {
			t.Errorf("awaited plain value = %v, want 5", v)
		}
	})
	order = append(order, "after-call")
	RunMicrotasks()
	want := []string{"start", "after-call", "resumed"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}
