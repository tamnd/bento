package value

import "testing"

// drainReset clears any microtasks a prior test left and runs the queue, so each
// test observes only its own callbacks. A test that enqueues nothing drains nothing.
func drainReset() {
	microtasks = nil
}

// TestResolvedThenDefersCallback proves a then callback on an already-resolved
// promise does not run inline: it is enqueued and fires only when the microtask
// queue drains, the ordering JavaScript gives even a settled promise.
func TestResolvedThenDefersCallback(t *testing.T) {
	drainReset()
	var order []string
	p := Resolved(7.0)
	p.Then(func(v float64) {
		order = append(order, "then")
		if v != 7 {
			t.Errorf("then value = %v, want 7", v)
		}
	})
	order = append(order, "after-then")
	RunMicrotasks()
	if len(order) != 2 || order[0] != "after-then" || order[1] != "then" {
		t.Errorf("callback ran inline, order = %v, want [after-then then]", order)
	}
}

// TestAsyncNormalReturnFulfills proves an await-free body that returns normally
// mints a fulfilled promise carrying its value.
func TestAsyncNormalReturnFulfills(t *testing.T) {
	drainReset()
	got := ""
	p := Async(func() float64 { return 21 })
	p.Then(func(v float64) {
		if v != 21 {
			t.Errorf("fulfilled value = %v, want 21", v)
		}
		got = "fulfilled"
	})
	p.Catch(func(Value) { got = "rejected" })
	RunMicrotasks()
	if got != "fulfilled" {
		t.Errorf("promise settled %q, want fulfilled", got)
	}
}

// TestAsyncThrowRejects proves a body that throws (panics with a Thrown, the way a
// bento throw raises) mints a rejected promise, and Catch receives the thrown value
// as a dynamic value whose message reads back.
func TestAsyncThrowRejects(t *testing.T) {
	drainReset()
	got := ""
	p := Async(func() float64 {
		Throw(NewError(FromGoString("boom")))
		return 0
	})
	p.Then(func(float64) { got = "fulfilled" })
	p.Catch(func(reason Value) {
		got = "rejected"
		if msg := reason.Get(FromGoString("message")).AsString().ToGoString(); msg != "boom" {
			t.Errorf("rejection message = %q, want boom", msg)
		}
	})
	RunMicrotasks()
	if got != "rejected" {
		t.Errorf("promise settled %q, want rejected", got)
	}
}

// TestAsyncVoidFulfills proves a void async body settles a unit promise on a normal
// return and rejects on a throw.
func TestAsyncVoidFulfills(t *testing.T) {
	drainReset()
	ran := false
	p := AsyncVoid(func() { ran = true })
	if !ran {
		t.Errorf("void async body did not run synchronously")
	}
	settled := ""
	p.Then(func(Unit) { settled = "fulfilled" })
	RunMicrotasks()
	if settled != "fulfilled" {
		t.Errorf("void promise settled %q, want fulfilled", settled)
	}

	drainReset()
	q := AsyncVoid(func() { Throw(NewError(FromGoString("x"))) })
	caught := false
	q.Catch(func(Value) { caught = true })
	RunMicrotasks()
	if !caught {
		t.Errorf("throwing void async body did not reject")
	}
}

// TestRunMicrotasksOrdersByEnqueue proves the queue runs callbacks in enqueue order
// and runs one a callback itself enqueues, the run-to-completion checkpoint.
func TestRunMicrotasksOrdersByEnqueue(t *testing.T) {
	drainReset()
	var order []int
	Resolved(1.0).Then(func(float64) {
		order = append(order, 1)
		Resolved(3.0).Then(func(float64) { order = append(order, 3) })
	})
	Resolved(2.0).Then(func(float64) { order = append(order, 2) })
	RunMicrotasks()
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("microtask order = %v, want [1 2 3]", order)
	}
}
