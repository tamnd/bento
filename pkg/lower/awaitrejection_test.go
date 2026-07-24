package lower

import (
	"strings"
	"testing"
)

// TestAwaitRejectionCatchesReason runs an async function that awaits a rejected promise
// inside a try and reads the caught reason: the rejection reason rides the throw into
// the catch, so `e === err` holds and the body reaches its console.log. Before the fix
// the caught binding was a fresh {name, message} object rather than the rejected value,
// so the identity check threw and the body never completed. This is the
// expressions/await/await-throws-rejections shape.
func TestAwaitRejectionCatchesReason(t *testing.T) {
	skipIfShort(t)
	const src = `async function foo(): Promise<void> {
  var err: any = {};
  var caught = false;
  try {
    await Promise.reject(err);
  } catch (e) {
    caught = true;
    if (e !== err) throw new Error("reason mismatch");
  }
  if (!caught) throw new Error("not caught");
  console.log("done");
}
foo();`
	if got, want := runProgramGo(t, src), "done\n"; !strings.Contains(got, want) {
		t.Fatalf("await-rejection test printed %q, want it to contain %q", got, want)
	}
}
