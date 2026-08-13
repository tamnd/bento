package value

import (
	"strings"
	"testing"
)

// TestCryptoValueIsOneObject pins the identity the whole slice rests on. A
// known-globals set collects the crypto global by identity, so a second read of the
// name has to be the same object the first read was, and so does the one the global
// object holds.
func TestCryptoValueIsOneObject(t *testing.T) {
	if !StrictEquals(CryptoValue(), CryptoValue()) {
		t.Error("crypto read twice gave two objects, want one")
	}
	if !StrictEquals(GlobalThisValue().Get(FromGoString("crypto")), CryptoValue()) {
		t.Error("globalThis.crypto is not the object the bare name reads")
	}
	if !StrictEquals(CryptoValue().Get(FromGoString("subtle")), CryptoValue().Get(FromGoString("subtle"))) {
		t.Error("crypto.subtle read twice gave two objects, want one")
	}
}

// TestCryptoCarriesTheThreeMembers pins the surface Crypto.prototype has in Node:
// subtle is an object and the other two are callable. A program reads these before
// it calls anything, since typeof is how a feature test spells the question.
func TestCryptoCarriesTheThreeMembers(t *testing.T) {
	c := CryptoValue()
	if got := c.TypeOf().ToGoString(); got != "object" {
		t.Errorf("typeof crypto is %q, want %q", got, "object")
	}
	if got := c.Get(FromGoString("subtle")).TypeOf().ToGoString(); got != "object" {
		t.Errorf("typeof crypto.subtle is %q, want %q", got, "object")
	}
	for _, name := range []string{"getRandomValues", "randomUUID"} {
		if got := c.Get(FromGoString(name)).TypeOf().ToGoString(); got != "function" {
			t.Errorf("typeof crypto.%s is %q, want %q", name, got, "function")
		}
	}
}

// TestRandomUUIDHasTheVersionFourShape pins the format rather than the bytes, which
// is all a caller can pin about a random value: the 8-4-4-4-12 grouping, the 4 the
// version nibble puts at index 14, and one of the four variant characters at 19.
func TestRandomUUIDHasTheVersionFourShape(t *testing.T) {
	id := randomUUIDString()
	if len(id) != 36 {
		t.Fatalf("randomUUID gave %q, want 36 characters", id)
	}
	groups := strings.Split(id, "-")
	want := []int{8, 4, 4, 4, 12}
	if len(groups) != len(want) {
		t.Fatalf("randomUUID gave %q, want five hyphen-separated groups", id)
	}
	for i, n := range want {
		if len(groups[i]) != n {
			t.Errorf("group %d of %q is %d characters, want %d", i, id, len(groups[i]), n)
		}
	}
	if id[14] != '4' {
		t.Errorf("randomUUID gave %q, want the version 4 nibble at index 14", id)
	}
	if !strings.ContainsRune("89ab", rune(id[19])) {
		t.Errorf("randomUUID gave %q, want an RFC 4122 variant character at index 19", id)
	}
	if strings.ToLower(id) != id {
		t.Errorf("randomUUID gave %q, want it lowercase", id)
	}
}

// TestRandomUUIDDoesNotRepeat pins that the source is entropy rather than a counter
// or a constant. A hundred draws colliding on a 122-bit space would mean the bytes
// are not random at all, so the count is an honest test even though the values are
// not fixed.
func TestRandomUUIDDoesNotRepeat(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := randomUUIDString()
		if seen[id] {
			t.Fatalf("randomUUID gave %q twice in a hundred draws", id)
		}
		seen[id] = true
	}
}

// TestGetRandomValuesFillsTheArrayItWasGiven pins both halves of what the member
// promises: the bytes under the view change, and the value handed back is the very
// array that was passed, which is what makes crypto.getRandomValues(a) === a hold.
func TestGetRandomValuesFillsTheArrayItWasGiven(t *testing.T) {
	a := NewUint8Array(32).ToValue()
	got := cryptoGetRandomValues(a)
	if !StrictEquals(got, a) {
		t.Error("getRandomValues answered a different array than it was given")
	}
	zeros := 0
	for i := range 32 {
		if ToNumber(got.GetIndex(float64(i))) == 0 {
			zeros++
		}
	}
	if zeros == 32 {
		t.Error("getRandomValues left every byte zero, want it filled from entropy")
	}
}

// TestGetRandomValuesRefusesAFloatArray pins the spec's first refusal. A float view
// is not an integer-type typed array, so the fill is a TypeMismatchError rather than
// a fill of bytes the caller would then read back as numbers it did not ask for.
func TestGetRandomValuesRefusesAFloatArray(t *testing.T) {
	for _, arg := range []Value{
		NewFloat64Array(4).ToValue(),
		NewFloat32Array(4).ToValue(),
		Number(5),
		NewArrayValue([]Value{Number(1), Number(2)}),
	} {
		name, msg := recoverThrown(t, func() { cryptoGetRandomValues(arg) })
		if name != "TypeMismatchError" {
			t.Errorf("getRandomValues of %s threw %q, want TypeMismatchError", arg.TypeOf().ToGoString(), name)
		}
		if msg != "The data argument must be an integer-type TypedArray" {
			t.Errorf("getRandomValues threw the message %q, want Node's", msg)
		}
	}
}

// TestGetRandomValuesRefusesPastTheQuota pins the spec's other refusal, and pins
// that the ceiling itself is inclusive: 65,536 bytes is served and one more is not.
func TestGetRandomValuesRefusesPastTheQuota(t *testing.T) {
	atLimit := NewUint8Array(cryptoMaxRandomBytes).ToValue()
	if !StrictEquals(cryptoGetRandomValues(atLimit), atLimit) {
		t.Error("getRandomValues refused a view exactly at the quota, want it served")
	}
	name, msg := recoverThrown(t, func() {
		cryptoGetRandomValues(NewUint8Array(cryptoMaxRandomBytes + 1).ToValue())
	})
	if name != "QuotaExceededError" {
		t.Errorf("getRandomValues past the quota threw %q, want QuotaExceededError", name)
	}
	if msg != "The requested length exceeds 65,536 bytes" {
		t.Errorf("getRandomValues past the quota threw the message %q, want Node's", msg)
	}
}

// TestSubtleCarriesItsMethodsAndRefusesEachOne pins the shape the gap is written
// as. Every SubtleCrypto method answers a promise and bento has no dynamic promise
// value, so the names are present, which keeps a typeof feature test reading
// "function" the way it does in Node, and calling one says why it cannot answer
// rather than handing back something that is not a promise.
func TestSubtleCarriesItsMethodsAndRefusesEachOne(t *testing.T) {
	s := CryptoValue().Get(FromGoString("subtle"))
	for _, name := range subtleCryptoNames {
		m := s.Get(FromGoString(name))
		if got := m.TypeOf().ToGoString(); got != "function" {
			t.Errorf("typeof crypto.subtle.%s is %q, want %q", name, got, "function")
		}
		errName, msg := recoverThrown(t, func() { m.Call() })
		if errName != "TypeError" {
			t.Errorf("crypto.subtle.%s threw %q, want a TypeError", name, errName)
		}
		if !strings.Contains(msg, "crypto.subtle."+name) {
			t.Errorf("crypto.subtle.%s threw %q, want it to name itself", name, msg)
		}
	}
}

// TestSubtleCoversTheWholeNodeSurface pins the list against the one Node carries on
// SubtleCrypto.prototype, so a name a program feature-tests for is present here
// rather than reading undefined where Node reads a function.
func TestSubtleCoversTheWholeNodeSurface(t *testing.T) {
	want := []string{
		"encrypt", "decrypt", "sign", "verify", "digest",
		"generateKey", "deriveKey", "deriveBits",
		"importKey", "exportKey", "wrapKey", "unwrapKey",
	}
	have := map[string]bool{}
	for _, name := range subtleCryptoNames {
		have[name] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("crypto.subtle is missing %s", name)
		}
	}
}

// TestDOMExceptionCarriesItsName pins the stand-in the abort pair and the crypto
// refusals share: bento has no DOMException type, so the exception's name rides on
// the runtime Error family, which is what a program branching on err.name reads.
func TestDOMExceptionCarriesItsName(t *testing.T) {
	e := newDOMException("TypeMismatchError", "the message")
	if got := e.ErrorName(); got != "TypeMismatchError" {
		t.Errorf("the exception's name is %q, want TypeMismatchError", got)
	}
	if got := e.ErrorMessage(); got != "the message" {
		t.Errorf("the exception's message is %q, want the message", got)
	}
	v := e.ToValue()
	if got := ToString(v.Get(FromGoString("name"))).ToGoString(); got != "TypeMismatchError" {
		t.Errorf("the boxed exception's name reads %q, want TypeMismatchError", got)
	}
}

// recoverThrown runs fn and reports the name and message of the error it threw. It
// fails the test when fn returns without throwing, which is what every caller here
// is checking against.
func recoverThrown(t *testing.T, fn func()) (name, message string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Error("the call did not throw")
			return
		}
		thrown, ok := r.(Thrown)
		if !ok {
			t.Errorf("the call threw %T, want a thrown value", r)
			return
		}
		name, message = thrown.ErrorName(), thrown.ErrorMessage()
	}()
	fn()
	return "", ""
}
