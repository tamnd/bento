package value

import "testing"

// TestCaughtRejectionPreservesReason pins that catching a rejected promise's reason
// hands back the very value the reject carried, not a {name, message} wrapper. A
// rejection wraps its reason in a Thrown whose ToValue is the original value; Caught
// must route that through the caught error's ToValue so `catch (e)` after
// `await Promise.reject(obj)` binds e to obj and `e === obj` holds. Before the fix the
// general Thrown case built a fresh {name, message} object and lost the reason, so the
// identity check failed. This is the expressions/await/await-throws-rejections shape.
func TestCaughtRejectionPreservesReason(t *testing.T) {
	obj := NewObject()
	caught := Caught(NewRejection(obj))
	if !StrictEquals(caught.ToValue(), obj) {
		t.Fatalf("caught rejection reason is not the original object: e === obj should hold")
	}
}

// TestCaughtRejectionPrimitivePreserved pins the same for a primitive reason: rejecting
// with a number binds the caught value to that number, so `e === 42` and typeof hold.
func TestCaughtRejectionPrimitivePreserved(t *testing.T) {
	caught := Caught(NewRejection(Number(42)))
	if !StrictEquals(caught.ToValue(), Number(42)) {
		t.Fatalf("caught rejection reason is not the original primitive")
	}
}
