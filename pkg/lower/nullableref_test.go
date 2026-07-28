package lower

import (
	"regexp"
	"strings"
	"testing"
)

// TestNullableRefFieldIsBarePointer pins that a `T | null` field whose T is a
// class lowers to the bare *T rather than a tagged sum. The tag would cost an
// allocation on every link and, worse, would break identity: the value read back
// out of the field would no longer be the pointer that was put in.
func TestNullableRefFieldIsBarePointer(t *testing.T) {
	const src = `class Link {
  next: Link | null = null;
  value: number = 0;
}
const head = new Link();
head.value = 1;
console.log(head.value);
`
	source := renderProgram(t, src)
	// The gap is gofmt's field alignment, so the field and its type are matched
	// with the whitespace left open.
	if !regexp.MustCompile(`Next\s+\*Link`).MatchString(source) {
		t.Errorf("nullable class field did not lower to a bare pointer:\n%s", source)
	}
}

// TestNullableRefNullCompareIsNil pins that comparing such a field against null
// lowers to the Go nil test rather than the boxed equality helper.
func TestNullableRefNullCompareIsNil(t *testing.T) {
	const src = `class Link {
  next: Link | null = null;
}
const head = new Link();
if (head.next === null) {
  console.log(0);
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "== nil") {
		t.Errorf("null compare did not lower to a nil test:\n%s", source)
	}
	if strings.Contains(source, "LooseEquals") {
		t.Errorf("null compare went through the boxed equality path:\n%s", source)
	}
}

// TestNullableRefLinkedListRuns builds and runs a linked list, which is the
// shape this lowering exists for and the shape the ES5-era benchmarks are built
// from: a `next: Link | null` field, a null-terminated walk, a store through the
// field, and a ternary that yields either a node or null. The list is built back
// to front and walked front to back, so a broken link would change the sum.
func TestNullableRefLinkedListRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Link {
  value: number;
  next: Link | null;
  constructor(value: number, next: Link | null) {
    this.value = value;
    this.next = next;
  }
}

function build(n: number): Link | null {
  let head: Link | null = null;
  for (let i = n; i > 0; i--) {
    head = new Link(i, head);
  }
  return head;
}

function sum(head: Link | null): number {
  let total = 0;
  let current = head;
  while (current !== null) {
    total = total + current.value;
    current = current.next;
  }
  return total;
}

const list = build(5);
console.log(sum(list));
// The ternary over a reference: both arms are the union's own Go type, so the
// null arm is nil and the node arm is the pointer itself.
const first: Link | null = list !== null ? list : null;
console.log(first === null ? 0 : first.value);
// A store through the field, which is what a list mutation does.
if (list !== null) {
  list.next = null;
}
console.log(sum(list));
`
	got := runProgramGo(t, src)
	const want = "15\n1\n1\n"
	if got != want {
		t.Fatalf("linked list printed %q, want %q", got, want)
	}
}

// TestNullableRefEmptyListRuns covers the null end of the shape on its own: a
// binding that starts null and is only filled later, so the first guard takes
// the null branch and nothing is ever dereferenced through it.
//
// The guards are statements rather than ternaries because a ternary over a
// binding the checker has narrowed to exactly `null` still hands back, which is
// a gap this change does not close: nullableRef wants the union, and at that
// point the type is the bare null.
func TestNullableRefEmptyListRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Link {
  next: Link | null = null;
}

function describe(head: Link | null): number {
  if (head === null) {
    return 1;
  }
  if (head.next === null) {
    return 2;
  }
  return 3;
}

console.log(describe(null));
const one = new Link();
console.log(describe(one));
const two = new Link();
two.next = one;
console.log(describe(two));
`
	got := runProgramGo(t, src)
	const want = "1\n2\n3\n"
	if got != want {
		t.Fatalf("empty list printed %q, want %q", got, want)
	}
}

// TestClassUpcastThisRuns covers the derived-to-base upcast in the three places
// the benchmark ports reach it: `this` handed to a function that takes the base,
// an element of an array of the base, and a `Base | null` field. Each one has to
// take the address of the embedded base rather than pass the derived pointer, so
// a miss is a compile error rather than a wrong answer, and reaching the base
// method through every one of them proves the address is the right one.
func TestClassUpcastThisRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Base {
  tag: number;
  constructor(tag: number) {
    this.tag = tag;
  }
  describe(): number {
    return this.tag;
  }
}

class Derived extends Base {
  extra: number;
  constructor(tag: number, extra: number) {
    super(tag);
    this.extra = extra;
  }
  register(): number {
    // this flowing into a base-typed parameter
    return take(this);
  }
}

function take(b: Base): number {
  return b.describe();
}

const d = new Derived(7, 9);
console.log(d.register());
// an array of the base holding a derived element
const all: Base[] = [];
all.push(d);
console.log(all[0].describe());
// a Base | null field holding a derived instance
class Holder {
  slot: Base | null = null;
}
const h = new Holder();
h.slot = d;
console.log(h.slot === null ? 0 : h.slot.describe());
console.log(d.extra);
`
	got := runProgramGo(t, src)
	const want = "7\n7\n7\n9\n"
	if got != want {
		t.Fatalf("class upcast printed %q, want %q", got, want)
	}
}
