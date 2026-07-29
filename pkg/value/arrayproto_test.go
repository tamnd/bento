package value

import (
	"sort"
	"strings"
	"testing"
)

// A boxed array used to answer undefined for every method name, so a program that
// called one invoked undefined and threw at run time, after the build had already
// succeeded. These cases pin that every named Array.prototype method is there and
// answers what Node answers.

// arr builds a boxed array of numbers, the receiver most of these read.
func arr(ns ...float64) Value {
	elems := make([]Value, len(ns))
	for i, n := range ns {
		elems[i] = Number(n)
	}
	return NewArrayValue(elems)
}

// join renders a boxed array the way a test can compare it, through the array's own
// join so the rendering is one of the things under test rather than a second opinion.
func join(v Value) string {
	return ToString(v.Get(FromGoString("join")).Call()).ToGoString()
}

// call reads a method off a boxed array and calls it, the exact two steps a dynamic
// `a.method(x)` takes: the read resolves through the prototype chain, and the result
// is invoked. A method that is missing fails at the call rather than the read, which
// is precisely the failure this file exists to prevent, so going through both steps is
// the point.
func call(t *testing.T, recv Value, name string, args ...Value) Value {
	t.Helper()
	m := recv.Get(FromGoString(name))
	if m.Kind() != KindFunc {
		t.Fatalf("%s read off an array is %v, want a function", name, m.Kind())
	}
	return m.Call(args...)
}

// nodeArrayProto is every named property Node reports for Array.prototype, taken from
// Object.getOwnPropertyNames(Array.prototype) on v24 with the two non-methods removed:
// length, which is a data property, and constructor, which is the Array function.
// Keeping the list here rather than deriving it from the table under test is what makes
// it a check: a method dropped from the table fails against Node's list rather than
// against a copy of itself.
var nodeArrayProto = []string{
	"at", "concat", "copyWithin", "entries", "every", "fill", "filter", "find",
	"findIndex", "findLast", "findLastIndex", "flat", "flatMap", "forEach", "includes",
	"indexOf", "join", "keys", "lastIndexOf", "map", "pop", "push", "reduce",
	"reduceRight", "reverse", "shift", "slice", "some", "sort", "splice",
	"toLocaleString", "toReversed", "toSorted", "toSpliced", "toString", "unshift",
	"values", "with",
}

// TestArrayProtoCoversNode pins the whole prototype rather than the methods a program
// happens to use. A subset is what left `map` missing in the first place, and a program
// finds the gap at run time, so the list is checked in both directions: nothing Node has
// is missing, and nothing is here that Node does not have.
func TestArrayProtoCoversNode(t *testing.T) {
	var got []string
	for name := range arrayProtoMethods {
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string(nil), nodeArrayProto...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Array.prototype methods are\n  %s\nwant\n  %s", strings.Join(got, ","), strings.Join(want, ","))
	}
}

// TestEveryProtoMethodIsCallable pins that each name in the table resolves to a callable
// off a real boxed array. The table could name a method the chain never reaches, which
// is the same run-time failure with a passing coverage test in front of it.
func TestEveryProtoMethodIsCallable(t *testing.T) {
	a := arr(1, 2, 3)
	for _, name := range nodeArrayProto {
		if m := a.Get(FromGoString(name)); m.Kind() != KindFunc {
			t.Errorf("%s off a boxed array is %v, want a function", name, m.Kind())
		}
	}
}

// TestTheReadersAnswerNode walks the methods that only read, each with the arguments and
// the answer Node gives.
func TestTheReadersAnswerNode(t *testing.T) {
	a := arr(1, 2, 3, 4)
	double := NewFunc(func(args []Value) Value { return Number(ToNumber(Arg(args, 0)) * 2) })
	isBig := NewFunc(func(args []Value) Value { return Bool(ToNumber(Arg(args, 0)) > 2) })
	sum := NewFunc(func(args []Value) Value {
		return Number(ToNumber(Arg(args, 0)) + ToNumber(Arg(args, 1)))
	})
	for _, c := range []struct {
		name string
		got  Value
		want string
	}{
		{"map", call(t, a, "map", double), "2,4,6,8"},
		{"filter", call(t, a, "filter", isBig), "3,4"},
		{"slice", call(t, a, "slice", Number(1), Number(3)), "2,3"},
		{"concat", call(t, a, "concat", arr(5)), "1,2,3,4,5"},
		{"reduce", call(t, a, "reduce", sum, Number(0)), "10"},
		{"reduceRight", call(t, a, "reduceRight", sum, Number(0)), "10"},
		{"find", call(t, a, "find", isBig), "3"},
		{"findIndex", call(t, a, "findIndex", isBig), "2"},
		{"findLast", call(t, a, "findLast", isBig), "4"},
		{"findLastIndex", call(t, a, "findLastIndex", isBig), "3"},
		{"indexOf", call(t, a, "indexOf", Number(3)), "2"},
		{"lastIndexOf", call(t, a, "lastIndexOf", Number(3)), "2"},
		{"includes", call(t, a, "includes", Number(3)), "true"},
		{"some", call(t, a, "some", isBig), "true"},
		{"every", call(t, a, "every", isBig), "false"},
		{"at", call(t, a, "at", Number(-1)), "4"},
		{"atPastEnd", call(t, a, "at", Number(9)), "undefined"},
		{"join", call(t, a, "join", StringValue(FromGoString("-"))), "1-2-3-4"},
		{"toString", call(t, a, "toString"), "1,2,3,4"},
		{"toLocaleString", call(t, a, "toLocaleString"), "1,2,3,4"},
		{"toReversed", call(t, a, "toReversed"), "4,3,2,1"},
		{"with", call(t, a, "with", Number(0), Number(9)), "9,2,3,4"},
		{"toSpliced", call(t, a, "toSpliced", Number(1), Number(2)), "1,4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := ToString(c.got).ToGoString(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	// The readers must not have touched the receiver, which is what separates them from
	// the mutators below and is easy to break when both are written the same way.
	if got := join(a); got != "1,2,3,4" {
		t.Errorf("a reader mutated the receiver, it is now %q", got)
	}
}

// TestFlatAndFlatMapAnswerNode pins the two that descend, including flatMap's rule that
// it flattens exactly one level whatever the callback returns.
func TestFlatAndFlatMapAnswerNode(t *testing.T) {
	nested := NewArrayValue([]Value{Number(1), NewArrayValue([]Value{Number(2), NewArrayValue([]Value{Number(3)})})})
	if got := ToString(call(t, nested, "flat")).ToGoString(); got != "1,2,3" {
		t.Errorf("flat() got %q, want %q", got, "1,2,3")
	}
	if got := ToString(call(t, nested, "flat", Number(2))).ToGoString(); got != "1,2,3" {
		t.Errorf("flat(2) got %q, want %q", got, "1,2,3")
	}
	pair := NewFunc(func(args []Value) Value {
		n := ToNumber(Arg(args, 0))
		return NewArrayValue([]Value{Number(n), Number(n * 10)})
	})
	if got := ToString(call(t, arr(1, 2), "flatMap", pair)).ToGoString(); got != "1,10,2,20" {
		t.Errorf("flatMap got %q, want %q", got, "1,10,2,20")
	}
}

// TestTheMutatorsChangeTheReceiver pins each in-place method by both halves: what it
// returns and what the receiver holds afterwards. A mutator that built a new array and
// returned the right thing would pass on the return alone, and every caller that kept a
// reference to the array would then see the old contents.
func TestTheMutatorsChangeTheReceiver(t *testing.T) {
	t.Run("push", func(t *testing.T) {
		a := arr(1, 2)
		if n := ToNumber(call(t, a, "push", Number(3), Number(4))); n != 4 {
			t.Errorf("push returned %v, want 4", n)
		}
		if got := join(a); got != "1,2,3,4" {
			t.Errorf("after push a is %q", got)
		}
	})
	t.Run("pop", func(t *testing.T) {
		a := arr(1, 2, 3)
		if v := ToNumber(call(t, a, "pop")); v != 3 {
			t.Errorf("pop returned %v, want 3", v)
		}
		if got := join(a); got != "1,2" {
			t.Errorf("after pop a is %q", got)
		}
	})
	t.Run("shift", func(t *testing.T) {
		a := arr(1, 2, 3)
		if v := ToNumber(call(t, a, "shift")); v != 1 {
			t.Errorf("shift returned %v, want 1", v)
		}
		if got := join(a); got != "2,3" {
			t.Errorf("after shift a is %q", got)
		}
	})
	t.Run("unshift", func(t *testing.T) {
		a := arr(3)
		if n := ToNumber(call(t, a, "unshift", Number(1), Number(2))); n != 3 {
			t.Errorf("unshift returned %v, want 3", n)
		}
		if got := join(a); got != "1,2,3" {
			t.Errorf("after unshift a is %q", got)
		}
	})
	t.Run("reverse", func(t *testing.T) {
		a := arr(1, 2, 3)
		call(t, a, "reverse")
		if got := join(a); got != "3,2,1" {
			t.Errorf("after reverse a is %q", got)
		}
	})
	t.Run("fill", func(t *testing.T) {
		a := arr(1, 2, 3)
		call(t, a, "fill", Number(0), Number(1))
		if got := join(a); got != "1,0,0" {
			t.Errorf("after fill a is %q", got)
		}
	})
	t.Run("copyWithin", func(t *testing.T) {
		a := arr(1, 2, 3, 4, 5)
		call(t, a, "copyWithin", Number(0), Number(3))
		if got := join(a); got != "4,5,3,4,5" {
			t.Errorf("after copyWithin a is %q", got)
		}
	})
}

// TestSpliceMovesElementsBothWays pins splice at the three lengths of item list, since
// each drives a different direction of shift and a wrong one silently overwrites an
// element it had not read.
func TestSpliceMovesElementsBothWays(t *testing.T) {
	for _, c := range []struct {
		name        string
		args        []Value
		wantRemoved string
		wantLeft    string
	}{
		{"shrink", []Value{Number(1), Number(2)}, "2,3", "1,4,5"},
		{"sameSize", []Value{Number(1), Number(2), Number(8), Number(9)}, "2,3", "1,8,9,4,5"},
		{"grow", []Value{Number(1), Number(1), Number(7), Number(8), Number(9)}, "2", "1,7,8,9,3,4,5"},
		{"toEnd", []Value{Number(2)}, "3,4,5", "1,2"},
		{"noArgs", nil, "", "1,2,3,4,5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := arr(1, 2, 3, 4, 5)
			removed := call(t, a, "splice", c.args...)
			if got := join(removed); got != c.wantRemoved {
				t.Errorf("splice returned %q, want %q", got, c.wantRemoved)
			}
			if got := join(a); got != c.wantLeft {
				t.Errorf("after splice a is %q, want %q", got, c.wantLeft)
			}
		})
	}
}

// TestSortComparesAsStringsByDefault pins the rule that surprises everyone and that a
// Go implementation is most likely to get wrong by sorting numerically: with no
// comparator the elements are ordered by their string forms, so 10 sorts before 9.
func TestSortComparesAsStringsByDefault(t *testing.T) {
	a := arr(10, 9, 1)
	call(t, a, "sort")
	if got := join(a); got != "1,10,9" {
		t.Errorf("sort with no comparator gave %q, want %q", got, "1,10,9")
	}
	b := arr(10, 9, 1)
	desc := NewFunc(func(args []Value) Value {
		return Number(ToNumber(Arg(args, 1)) - ToNumber(Arg(args, 0)))
	})
	call(t, b, "sort", desc)
	if got := join(b); got != "10,9,1" {
		t.Errorf("sort with a comparator gave %q, want %q", got, "10,9,1")
	}
	c := arr(10, 9, 1)
	if got := join(call(t, c, "toSorted")); got != "1,10,9" {
		t.Errorf("toSorted gave %q, want %q", got, "1,10,9")
	}
	if got := join(c); got != "10,9,1" {
		t.Errorf("toSorted mutated its receiver, it is now %q", got)
	}
}

// TestSortRejectsANonFunctionComparator pins the check that runs before anything is
// read, so sort(1) throws rather than quietly sorting by string.
func TestSortRejectsANonFunctionComparator(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("sort(1) did not throw")
		}
	}()
	call(t, arr(1, 2), "sort", Number(1))
}

// TestWithRejectsAnIndexOutsideTheArray pins the difference between with and a plain
// write: an out-of-range index throws rather than growing the result.
func TestWithRejectsAnIndexOutsideTheArray(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("with(5, x) on a three-element array did not throw")
		}
	}()
	call(t, arr(1, 2, 3), "with", Number(5), Number(0))
}

// TestTheIteratorsWalkTheArray pins keys, values and entries as real iterator objects: a
// next() answering { value, done }, and a Symbol.iterator answering itself so the result
// can be handed to anything that takes an iterable.
func TestTheIteratorsWalkTheArray(t *testing.T) {
	a := arr(7, 8)
	for _, c := range []struct{ name, want string }{
		{"keys", "0|1"},
		{"values", "7|8"},
		{"entries", "0,7|1,8"},
	} {
		t.Run(c.name, func(t *testing.T) {
			it := call(t, a, c.name)
			next := it.Get(FromGoString("next"))
			var got []string
			for {
				r := next.Call()
				if ToBoolean(r.Get(FromGoString("done"))) {
					break
				}
				got = append(got, ToString(r.Get(FromGoString("value"))).ToGoString())
			}
			if strings.Join(got, "|") != c.want {
				t.Errorf("%s walked %q, want %q", c.name, strings.Join(got, "|"), c.want)
			}
			if self := it.getSymKey(symbolIterator); self.Kind() != KindFunc {
				t.Errorf("%s result has no Symbol.iterator", c.name)
			}
		})
	}
}

// TestAnOwnPropertyStillWinsOverTheProtoMethod pins the lookup order the whole design
// rests on. The prototype answer runs only on a genuine chain miss, so a program that
// stored its own `map` on an array reads that one back, the way own-before-inherited
// lookup requires.
func TestAnOwnPropertyStillWinsOverTheProtoMethod(t *testing.T) {
	a := arr(1, 2)
	a.Set(FromGoString("map"), Number(42))
	if got := ToNumber(a.Get(FromGoString("map"))); got != 42 {
		t.Errorf("an own map read back as %v, want the 42 that was stored", got)
	}
}

// TestAnArrayStillInheritsTheObjectMethods pins that adding the array prototype did not
// shadow Object.prototype: a name only Object.prototype carries still resolves, and one
// both carry resolves to the array's, which is what the chain order requires.
func TestAnArrayStillInheritsTheObjectMethods(t *testing.T) {
	a := arr(1, 2)
	if m := a.Get(FromGoString("hasOwnProperty")); m.Kind() != KindFunc {
		t.Fatalf("hasOwnProperty off an array is %v, want a function", m.Kind())
	}
	if !ToBoolean(a.Get(FromGoString("hasOwnProperty")).Call(StringValue(FromGoString("0")))) {
		t.Error("hasOwnProperty('0') on a two-element array said false")
	}
	// Object.prototype.toLocaleString would answer through the receiver's toString; the
	// array's own answers by rendering each element. Both give "1,2" here, so the check
	// that the array's won is that it is the array table that supplies it.
	if _, ok := arrayProtoMethods["toLocaleString"]; !ok {
		t.Error("toLocaleString is not in the array table, so Object.prototype's would win")
	}
}

// TestTheMethodsWorkOnAnArrayLike pins the other half of delegating to the generic
// implementations: the same methods run on a plain object carrying a length and integer
// keys, which is what a borrowed Array.prototype.<m>.call does and what makes the
// arguments object and a DOM-style collection work.
func TestTheMethodsWorkOnAnArrayLike(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("0"), Number(5))
	o.Set(FromGoString("1"), Number(6))
	o.Set(FromGoString("length"), Number(2))
	if got := ToString(GenericJoin(o)).ToGoString(); got != "5,6" {
		t.Errorf("join on an array-like gave %q, want %q", got, "5,6")
	}
	GenericPush(o, Number(7))
	if got := ToNumber(o.Get(FromGoString("length"))); got != 3 {
		t.Errorf("after push the array-like length is %v, want 3", got)
	}
	if got := ToNumber(o.Get(FromGoString("2"))); got != 7 {
		t.Errorf("after push index 2 is %v, want 7", got)
	}
}

// TestANullishReceiverThrows pins the ToObject step every generic method takes first, so
// a borrowed call on null throws the TypeError Node throws rather than reading a zero
// length and answering quietly.
func TestANullishReceiverThrows(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("map on a null receiver did not throw")
		}
	}()
	GenericMap(Null, NewFunc(func(args []Value) Value { return Undefined }))
}
