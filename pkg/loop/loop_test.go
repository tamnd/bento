package loop

import (
	"testing"
	"time"

	"github.com/tamnd/bento/pkg/engine"
)

// fakeEngine records the order in which timers fire. It ignores everything
// except Call, which the loop uses to dispatch a due timer.
type fakeEngine struct {
	fired []int64
}

func (f *fakeEngine) Name() string                           { return "fake" }
func (f *fakeEngine) Eval(string, string) (any, error)       { return nil, nil }
func (f *fakeEngine) EvalModule(string, string) error        { return nil }
func (f *fakeEngine) Register(string, engine.HostFunc) error { return nil }
func (f *fakeEngine) DrainMicrotasks() (int, error)          { return 0, nil }
func (f *fakeEngine) SetModuleLoader(engine.ModuleLoader)    {}
func (f *fakeEngine) SetModuleHost(engine.ModuleHost)        {}
func (f *fakeEngine) Interrupt()                             {}
func (f *fakeEngine) Close() error                           { return nil }
func (f *fakeEngine) Call(fn string, args ...any) (any, error) {
	if fn == "__bento_runTimer" && len(args) > 0 {
		f.fired = append(f.fired, args[0].(int64))
	}
	return nil, nil
}

func TestTimersFireInDueOrder(t *testing.T) {
	f := &fakeEngine{}
	l := New(f)
	// Register out of order; they must fire by due time, not insertion order.
	l.AddTimer(1, 30, false)
	l.AddTimer(2, 10, false)
	l.AddTimer(3, 20, false)

	if err := l.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []int64{2, 3, 1}
	if len(f.fired) != len(want) {
		t.Fatalf("fired %v, want %v", f.fired, want)
	}
	for i := range want {
		if f.fired[i] != want[i] {
			t.Fatalf("fired %v, want %v", f.fired, want)
		}
	}
}

func TestClearTimerCancels(t *testing.T) {
	f := &fakeEngine{}
	l := New(f)
	l.AddTimer(1, 10, false)
	l.AddTimer(2, 20, false)
	l.ClearTimer(2)

	if err := l.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(f.fired) != 1 || f.fired[0] != 1 {
		t.Fatalf("fired %v, want [1]", f.fired)
	}
}

func TestPostRunsTaskOnLoop(t *testing.T) {
	// A task posted from another goroutine runs on the loop goroutine. A handle
	// reference keeps the loop alive until the task arrives, then the task drops
	// the reference so the loop can exit.
	f := &fakeEngine{}
	l := New(f)
	l.AddRef()

	ran := make(chan struct{})
	go func() {
		l.Post(func() {
			close(ran)
			l.Unref()
		})
	}()

	done := make(chan error, 1)
	go func() { done <- l.Run() }()

	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("posted task never ran")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("loop should exit after the last ref drops")
	}
}

func TestPostWakesLoopWaitingOnTimer(t *testing.T) {
	// A task posted while the loop waits on a far-off timer runs promptly rather
	// than after the timer's full delay.
	f := &fakeEngine{}
	l := New(f)
	l.AddTimer(1, 10_000, false) // ten seconds away

	ran := make(chan struct{})
	go func() {
		l.Post(func() {
			close(ran)
			l.ClearTimer(1)
			l.Stop()
		})
	}()

	done := make(chan error, 1)
	go func() { done <- l.Run() }()

	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("post did not wake the loop before the timer")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("loop should stop after the task")
	}
}

func TestEmptyLoopReturns(t *testing.T) {
	f := &fakeEngine{}
	l := New(f)
	done := make(chan error, 1)
	go func() { done <- l.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("empty loop should return immediately")
	}
}
