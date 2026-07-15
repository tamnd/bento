package lower

import (
	"strings"
	"testing"
)

// The general in operator, key in obj, is a runtime property-existence check on an
// object value, distinct from the discriminated-union tag test #237 folds a narrowing
// in to. These tests pin the forms value.InOperator answers: a string, number, symbol,
// and dynamic key, the prototype-chain walk, a non-enumerable property, the TypeError a
// primitive receiver raises, and the handback a static fixed-shape receiver keeps until
// object boxing lands.

// TestInStringKeyRuns proves a string key reads own-property existence on a dynamic
// object, true for a present property and false for an absent one.
func TestInStringKeyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const o: any = { a: 1 };\nconsole.log(\"a\" in o);\nconsole.log(\"b\" in o);\n"
	want := "true\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("string-key in printed %q, want %q", got, want)
	}
}

// TestInNumberKeyRuns proves a numeric key coerces to its property-key string, so
// 1 in arr reads the "1" index, clearing the coerce-the-key item.
func TestInNumberKeyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const a: any = [10, 20, 30];\nconsole.log(1 in a);\nconsole.log(5 in a);\n"
	want := "true\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("number-key in printed %q, want %q", got, want)
	}
}

// TestInDynamicKeyRuns proves a dynamic key, whose static type is any, coerces through
// ToPropertyKey at run time and reaches the same existence check a string literal does.
func TestInDynamicKeyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const o: any = { a: 1 };\nconst k: any = \"a\";\nconsole.log(k in o);\n"
	want := "true\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("dynamic-key in printed %q, want %q", got, want)
	}
}

// TestInSymbolKeyRuns proves a symbol key is probed by identity, present only for the
// exact symbol the object was keyed with.
func TestInSymbolKeyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const s = Symbol(\"k\");\nconst o: any = {};\no[s] = 1;\nconsole.log(s in o);\nconsole.log(Symbol(\"k\") in o);\n"
	want := "true\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("symbol-key in printed %q, want %q", got, want)
	}
}

// TestInWalksPrototypeChain proves the check climbs the prototype chain, so a property
// defined on a parent object reads as present through a child created with it as its
// prototype, while a property on neither reads absent.
func TestInWalksPrototypeChain(t *testing.T) {
	skipIfShort(t)
	const src = "const base: any = { inherited: 1 };\nconst derived: any = Object.create(base);\nderived.own = 2;\nconsole.log(\"own\" in derived);\nconsole.log(\"inherited\" in derived);\nconsole.log(\"missing\" in derived);\n"
	want := "true\ntrue\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("prototype-chain in printed %q, want %q", got, want)
	}
}

// TestInSeesNonEnumerable proves the check sees a non-enumerable own property, which a
// for-in key enumeration skips, so `"hidden" in o` reports true while iterating the keys
// yields nothing.
func TestInSeesNonEnumerable(t *testing.T) {
	skipIfShort(t)
	const src = "const o: any = {};\nObject.defineProperty(o, \"hidden\", { value: 42, enumerable: false });\nlet keys = \"\";\nfor (const k in o) { keys += k; }\nconsole.log(\"hidden\" in o);\nconsole.log(\"keys=[\" + keys + \"]\");\n"
	want := "true\nkeys=[]\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("non-enumerable in printed %q, want %q", got, want)
	}
}

// TestInObjectLiteralReceiverRuns proves an object literal in the receiver position
// boxes into a live object the check reads, so "a" in { a: 1 } lowers even though the
// literal's own type is a fixed shape.
func TestInObjectLiteralReceiverRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(\"a\" in { a: 1 });\nconsole.log(\"b\" in { a: 1 });\n"
	want := "true\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("object-literal-receiver in printed %q, want %q", got, want)
	}
}

// TestInPrimitiveReceiverThrows proves a primitive right operand raises a TypeError, the
// non-object rule, rather than answering off the primitive's own carried properties.
func TestInPrimitiveReceiverThrows(t *testing.T) {
	skipIfShort(t)
	const src = "const n: any = 5;\ntry { \"x\" in n; console.log(\"no throw\"); } catch (e) { console.log(e instanceof TypeError); }\n"
	want := "true\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("primitive-receiver in printed %q, want %q", got, want)
	}
}

// TestInRoutesThroughInOperator pins that the general form emits value.InOperator, the
// runtime existence check, not the tag disjunction the union-narrowing case folds to.
func TestInRoutesThroughInOperator(t *testing.T) {
	const src = "const o: any = { a: 1 };\nconsole.log(\"a\" in o);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.InOperator") {
		t.Fatalf("general in did not route through value.InOperator:\n%s", source)
	}
}

// TestInStaticShapeReceiverHandsBack pins that a static fixed-shape object binding, which
// has no box yet, hands the whole in back rather than emit Go that cannot answer runtime
// property existence, keeping the zero-fail invariant until object boxing lands.
func TestInStaticShapeReceiverHandsBack(t *testing.T) {
	const src = "const o = { a: 1 };\nconsole.log(\"a\" in o);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "in operator outside a discriminated-union narrowing") {
		t.Fatalf("static-shape receiver handed back with %q, want the in-operator reason", reason)
	}
}
