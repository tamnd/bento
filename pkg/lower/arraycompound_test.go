package lower

import (
	"strings"
	"testing"
)

// TestNewArraySizedRuns covers `new Array<T>(n)`, the sized-array constructor a
// numeric kernel opens with. It has to produce n elements at T's zero value, not
// an empty array, so writing every slot back to front and reading it forward
// proves both the length and the initial contents.
func TestNewArraySizedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const xs = new Array<number>(4);
console.log(xs.length);
console.log(xs[0]);
for (let i = 3; i >= 0; i--) {
  xs[i] = i * 10;
}
let total = 0;
for (let i = 0; i < xs.length; i++) {
  total = total + xs[i];
}
console.log(total);
`
	got := runProgramGo(t, src)
	const want = "4\n0\n60\n"
	if got != want {
		t.Fatalf("sized array printed %q, want %q", got, want)
	}
}

// TestCompoundArrayWriteRepeatsIndex pins the reason repeatableOperand had to
// recurse over the operator forms rather than stop at a property read: a
// compound element write names its index twice, once to read and once to store,
// so an index like `a[this.t - 1]` has to be admitted as repeatable or the write
// hands back. It is only repeatable because both operands are, which is what the
// recursion establishes.
func TestCompoundArrayWriteRepeatsIndex(t *testing.T) {
	skipIfShort(t)
	// The class is named Frame rather than Buffer because Buffer is a declared global
	// now, and a top-level class in a script shares the global scope with it. That is
	// TypeScript's own rule against @types/node, not a bento restriction: the same
	// declaration inside a module has its own scope and does not collide.
	const src = `class Frame {
  data: number[];
  t: number;
  constructor() {
    this.data = [1, 2, 3, 4];
    this.t = 3;
  }
  bump(): void {
    this.data[this.t - 1] += 10;
    this.data[this.t] -= 1;
  }
}
const b = new Frame();
b.bump();
console.log(b.data[2]);
console.log(b.data[3]);
`
	got := runProgramGo(t, src)
	const want = "13\n3\n"
	if got != want {
		t.Fatalf("compound element write printed %q, want %q", got, want)
	}
}

// TestIncrementArrayElementRuns covers `++a[i]` and `a[i]--` as statements,
// which lower to the same read-combine-store the compound write emits. An index
// with its own arithmetic proves the element is addressed the same way on both
// halves of the store, and stepping two different elements proves the store
// lands where the read came from.
//
// Value position, `console.log(++a[i])`, still hands back. The statement is what
// a loop body writes and what the benchmark ports needed.
func TestIncrementArrayElementRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [1, 2, 3];
const i = 0;
++a[i + 1];
++a[i + 1];
a[2]--;
--a[0];
console.log(a[0]);
console.log(a[1]);
console.log(a[2]);
`
	got := runProgramGo(t, src)
	const want = "0\n4\n2\n"
	if got != want {
		t.Fatalf("element increment printed %q, want %q", got, want)
	}
}

// TestIncrementIsNotRepeatable pins the other half of the repeatableOperand
// change: `++i` was being called repeatable, which it never was, since naming it
// twice would increment twice. A compound element write names its index once to
// read and once to store, so with `++i` as the index the only correct answers
// are a temporary or a hand-back. It hands back, with the reason saying so,
// which is the boundary to keep until the temporary exists.
func TestIncrementIsNotRepeatable(t *testing.T) {
	const src = `const a: number[] = [0, 0, 0, 0];
let i = 0;
a[++i] += 5;
console.log(a[1]);
console.log(i);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "side-effecting") {
		t.Errorf("compound write with an incrementing index handed back for %q, want the side-effecting index rule", reason)
	}
}

// TestNonNullAssertionRuns covers the `!` operator over both shapes it has to
// reach through: a nullable reference, where the assertion is a no-op because
// the union and the narrowed type share one pointer, and an optional element
// read, where it unwraps the value.Opt. `as` over the same optional takes the
// same path.
func TestNonNullAssertionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Link {
  value: number;
  constructor(value: number) {
    this.value = value;
  }
}

function first(items: Link[]): Link | null {
  if (items.length === 0) {
    return null;
  }
  return items[0];
}

const items: Link[] = [new Link(4), new Link(5)];
// ! over a nullable reference
console.log(first(items)!.value);
// ! over the optional an out-of-range-capable read yields
const popped = items.pop()!;
console.log(popped.value);
console.log(items.length);
`
	got := runProgramGo(t, src)
	const want = "4\n5\n1\n"
	if got != want {
		t.Fatalf("non-null assertion printed %q, want %q", got, want)
	}
}
