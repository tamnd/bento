package value

import (
	"testing"
)

// A named write to a primitive boxes it to a throwaway wrapper and discards it, so a
// sloppy assignment is a silent no-op and must not dereference the primitive's ref as
// an *Object. This is the path sym.a = 0 (and the number/string/boolean/bigint forms)
// lower to outside strict mode; before the guard it nil-panicked.
func TestSetOnPrimitiveIsSilentNoOp(t *testing.T) {
	for _, tc := range []struct {
		name string
		recv Value
	}{
		{"symbol", NewSymbolNoDesc()},
		{"number", Number(1)},
		{"string", StringValue(FromGoString("foo"))},
		{"boolean", True},
		{"bigint", BigIntFromInt64(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("sloppy Set on %s panicked: %v", tc.name, r)
				}
			}()
			got := tc.recv.Set(FromGoString("a"), Number(0))
			if got.kind != tc.recv.kind {
				t.Fatalf("Set on %s returned kind %v, want the receiver unchanged", tc.name, got.kind)
			}
		})
	}
}

// A strict-mode named write to a primitive throws a TypeError rather than dropping the
// write, the semantics assert.throws(TypeError, ...) pins for auto-boxing-strict.js.
func TestSetStrictOnPrimitiveThrowsTypeError(t *testing.T) {
	for _, tc := range []struct {
		name string
		recv Value
	}{
		{"symbol", NewSymbolNoDesc()},
		{"number", Number(1)},
		{"string", StringValue(FromGoString("foo"))},
		{"boolean", True},
		{"bigint", BigIntFromInt64(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("strict Set on %s did not throw", tc.name)
				}
				e, ok := r.(*Error)
				if !ok {
					t.Fatalf("thrown value is %T, want *Error", r)
				}
				if !e.IsA("TypeError") {
					t.Fatalf("thrown error is %s, want TypeError", e.name.ToGoString())
				}
			}()
			tc.recv.SetStrict(FromGoString("a"), Number(0))
		})
	}
}

// A strict computed or numeric bracket write to a primitive throws the same TypeError
// its named counterpart does, the semantics auto-boxing-strict.js pins for
// sym['a'+'b'] = 0 and sym[62] = 0, which lower to SetKeyStrict / SetIndexStrict.
func TestSetKeyStrictOnPrimitiveThrowsTypeError(t *testing.T) {
	sym := NewSymbolNoDesc()
	for _, name := range []string{"computed-key", "numeric-index"} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("strict %s write to a symbol did not throw", name)
				}
				if e, ok := r.(*Error); !ok || !e.IsA("TypeError") {
					t.Fatalf("thrown value = %v, want *Error TypeError", r)
				}
			}()
			if name == "numeric-index" {
				sym.SetIndexStrict(62, Number(0))
			} else {
				sym.SetKeyStrict(FromGoString("ab"), Number(0))
			}
		})
	}
}
