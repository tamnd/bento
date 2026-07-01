package engine

import (
	"errors"
	"testing"
)

// stubEngine is a do-nothing Engine used to exercise the registry.
type stubEngine struct{ name string }

func (s *stubEngine) Name() string                     { return s.name }
func (s *stubEngine) Eval(string, string) (any, error) { return nil, nil }
func (s *stubEngine) EvalModule(string, string) error  { return nil }
func (s *stubEngine) Call(string, ...any) (any, error) { return nil, nil }
func (s *stubEngine) Register(string, HostFunc) error  { return nil }
func (s *stubEngine) DrainMicrotasks() (int, error)    { return 0, nil }
func (s *stubEngine) SetModuleLoader(ModuleLoader)     {}
func (s *stubEngine) Interrupt()                       {}
func (s *stubEngine) Close() error                     { return nil }

func TestRegisterAndNew(t *testing.T) {
	Register("stub-test", func() (Engine, error) { return &stubEngine{name: "stub-test"}, nil })

	eng, err := New("stub-test")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if eng.Name() != "stub-test" {
		t.Errorf("want stub-test, got %q", eng.Name())
	}
}

func TestFirstRegistrationBecomesDefault(t *testing.T) {
	// Some backend must have claimed the default by the time any test runs.
	// Registering another one here must not steal it.
	before := Default()
	Register("stub-default-probe", func() (Engine, error) { return &stubEngine{}, nil })
	if Default() != before {
		t.Errorf("default changed from %q to %q on a later registration", before, Default())
	}
}

func TestNewUnknownBackend(t *testing.T) {
	_, err := New("does-not-exist")
	if err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

func TestSetDefaultUnknown(t *testing.T) {
	if err := SetDefault("also-missing"); err == nil {
		t.Fatal("expected an error setting an unknown default")
	}
}

func TestExceptionErrorsAs(t *testing.T) {
	var err error = &Exception{Message: "Error: x"}
	var ex *Exception
	if !errors.As(err, &ex) {
		t.Fatal("Exception should satisfy errors.As")
	}
	if ex.Display() == "" {
		t.Error("Display should be non-empty")
	}
}
