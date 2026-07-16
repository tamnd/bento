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

// TestInStaticShapeRequiredFoldsTrue pins that a required own property on a static
// fixed-shape binding folds "key" in obj to the constant true, the value the boxing
// InOperator would answer without a box, since a required property is always present.
func TestInStaticShapeRequiredFoldsTrue(t *testing.T) {
	skipIfShort(t)
	const src = "const o = { a: 1, b: 2 };\nconsole.log(\"a\" in o);\n"
	want := "true\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("required-member in printed %q, want %q", got, want)
	}
	source := renderProgram(t, src)
	if strings.Contains(source, "value.InOperator") {
		t.Fatalf("required-member in should fold to a constant, not route through InOperator:\n%s", source)
	}
}

// TestInStaticShapeRequiredMethodFoldsTrue pins that a class method, a property on the
// instance's prototype rather than an own field, still folds to true, the membership a
// required method always answers.
func TestInStaticShapeRequiredMethodFoldsTrue(t *testing.T) {
	skipIfShort(t)
	const src = "class C { m(): number { return 1; } }\nconst c = new C();\nconsole.log(\"m\" in c);\n"
	want := "true\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("required-method in printed %q, want %q", got, want)
	}
}

// TestInStaticShapeOptionalHandsBack pins that an optional member does not fold, since it
// may be absent, so the whole in hands back rather than emit an unsound true.
func TestInStaticShapeOptionalHandsBack(t *testing.T) {
	const src = "const o: { x?: number } = {};\nconsole.log(\"x\" in o);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "in operator outside a discriminated-union narrowing") {
		t.Fatalf("optional-member in handed back with %q, want the in-operator reason", reason)
	}
}

// TestInStaticShapeAbsentHandsBack pins that a member the shape does not declare does not
// fold to false, since JavaScript may still find it on Object.prototype, so a name like
// toString keeps the honest handback rather than fold to an unsound false.
func TestInStaticShapeAbsentHandsBack(t *testing.T) {
	for _, key := range []string{"z", "toString"} {
		src := "const o = { a: 1 };\nconsole.log(\"" + key + "\" in o);\n"
		reason := renderProgramHandBack(t, src)
		if !strings.Contains(reason, "in operator outside a discriminated-union narrowing") {
			t.Fatalf("absent-member %q in handed back with %q, want the in-operator reason", key, reason)
		}
	}
}

// TestInStaticShapeSideEffectingReceiverHandsBack pins that a receiver with a side effect
// does not fold, since folding drops the receiver and would lose its effect, so a call
// receiver keeps the handback.
func TestInStaticShapeSideEffectingReceiverHandsBack(t *testing.T) {
	const src = "function mk() { return { x: 1 }; }\nconsole.log(\"x\" in mk());\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "in operator outside a discriminated-union narrowing") {
		t.Fatalf("side-effecting-receiver in handed back with %q, want the in-operator reason", reason)
	}
}
