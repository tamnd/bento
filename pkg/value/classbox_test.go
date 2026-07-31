package value

import "testing"

// A class instance crossing into a dynamic slot boxes into a plain object carrying its
// fields under a prototype interned for the class. These hold what that prototype buys:
// the class's name in every rendering position, and a strict deep comparison that tells
// two classes apart and tells a class from a plain object with the same fields. Every
// expectation is Node v24's.

// pointClass and its siblings stand in for what the lowerer emits for a class: a struct
// with one exported field per source field carrying the property name in a json tag. The
// registrations sit in an init rather than on the vars, which is how the emitted code
// spells them, because a test file's vars would run before the ones the runtime declares.
type pointClass struct {
	X float64 `json:"x"`
	Y BStr    `json:"y"`
}

// twinClass has pointClass's exact fields and a different name, which is the pair Node's
// strict comparison separates and its loose comparison does not.
type twinClass struct {
	X float64 `json:"x"`
	Y BStr    `json:"y"`
}

// emptyClass has no fields, the instance that renders as a bare name and braces.
type emptyClass struct{}

// nestedClass holds another instance, the position that has no boxing site of its own:
// the inner one is reached by the reflection walk and can only be named by the registry.
type nestedClass struct {
	Inner *pointClass `json:"inner"`
}

// BaseClass and derivedClass are a hierarchy. The base type is exported because the
// embedded field takes its name and an unexported one would be machinery the reflection
// walk skips, which is how the lowerer emits it: the derived struct embeds the base so
// Go promotion serves the inherited fields and the walk flattens them in base-first.
type BaseClass struct {
	A float64 `json:"a"`
}

type derivedClass struct {
	BaseClass
	B float64 `json:"b"`
}

// privateClass carries an unexported field, which is what a #field lowers to.
type privateClass struct {
	p_secret float64 //nolint:unused // read by the reflection walk's exported-field test, not by Go
	Shown    float64 `json:"shown"`
}

func init() {
	RegisterClass("Point", (*pointClass)(nil))
	RegisterClass("Twin", (*twinClass)(nil))
	RegisterClass("Empty", (*emptyClass)(nil))
	RegisterClass("Nested", (*nestedClass)(nil))
	RegisterClass("Base", (*BaseClass)(nil))
	RegisterClass("Derived", (*derivedClass)(nil))
	RegisterClass("Private", (*privateClass)(nil))
}

// newPoint is the instance most of these tests box.
func newPoint() *pointClass {
	return &pointClass{X: 1, Y: FromGoString("s")}
}

// TestAClassInstanceBoxesItsFields holds the base case: the box is an ordinary object
// whose own enumerable keys are the class's fields, in declaration order, under the
// source property names rather than the Go field names.
func TestAClassInstanceBoxesItsFields(t *testing.T) {
	box := ObjectFromStruct(newPoint())

	if got := keyList(box.OwnEnumerableKeys()); got != "x,y" {
		t.Errorf("the boxed instance's own keys = %q, want \"x,y\"", got)
	}
	if got := box.Get(FromGoString("x")).AsNumber(); got != 1 {
		t.Errorf("the boxed .x = %v, want 1", got)
	}
	if got := box.Get(FromGoString("y")).AsString().ToGoString(); got != "s" {
		t.Errorf("the boxed .y = %q, want \"s\"", got)
	}
}

// TestAClassInstanceCarriesItsPrototype holds the thing everything else hangs off. The
// prototype is interned per class, so two instances of one class share one, and it
// carries the constructor Node's renderer and its comparison both read.
func TestAClassInstanceCarriesItsPrototype(t *testing.T) {
	a := ObjectFromStruct(newPoint())
	b := ObjectFromStruct(&pointClass{X: 2})

	pa, pb := a.object().proto, b.object().proto
	if pa == nil {
		t.Fatal("a boxed class instance carries no prototype")
	}
	if pa != pb {
		t.Error("two instances of one class carry different prototypes")
	}
	if pc := ObjectFromStruct(&twinClass{}).object().proto; pc == pa {
		t.Error("two different classes share one prototype")
	}

	ctor := a.Get(FromGoString("constructor"))
	if ctor.Kind() != KindFunc {
		t.Fatalf("the prototype's constructor is %s, want a function", kindName(ctor))
	}
	if got := ToString(ctor.Get(FromGoString("name"))).ToGoString(); got != "Point" {
		t.Errorf("the constructor's name = %q, want \"Point\"", got)
	}
}

// TestAClassInstanceDoesNotOwnItsConstructor holds the prototype's one property out of
// every own-key walk, which is where JavaScript keeps it: Object.keys of an instance is
// its fields, and JSON.stringify of one does not grow a constructor field.
func TestAClassInstanceDoesNotOwnItsConstructor(t *testing.T) {
	box := ObjectFromStruct(newPoint())

	if _, ok := box.object().getOwnDesc(FromGoString("constructor")); ok {
		t.Error("the boxed instance owns a constructor property")
	}
	if !box.HasProperty(FromGoString("constructor")) {
		t.Error("the boxed instance cannot reach the constructor on its prototype")
	}
	if got := JSONStringify(box).ToGoString(); got != `{"x":1,"y":"s"}` {
		t.Errorf("JSON.stringify of a boxed instance = %s, want {\"x\":1,\"y\":\"s\"}", got)
	}
}

// TestAClassInstanceRendersUnderItsName covers the rendering positions. Each is Node's
// output whole, including the nested one, where the inner instance is reached by
// reflection rather than by a boxing site and is named all the same.
func TestAClassInstanceRendersUnderItsName(t *testing.T) {
	cases := []struct {
		name string
		box  Value
		want string
	}{
		{"a plain instance", ObjectFromStruct(newPoint()), "Point { x: 1, y: 's' }"},
		{"an instance with no fields", ObjectFromStruct(&emptyClass{}), "Empty {}"},
		{"an instance holding another", ObjectFromStruct(&nestedClass{Inner: newPoint()}), "Nested { inner: Point { x: 1, y: 's' } }"},
		{"a derived instance", ObjectFromStruct(&derivedClass{BaseClass: BaseClass{A: 1}, B: 2}), "Derived { a: 1, b: 2 }"},
		{"an instance with a private field", ObjectFromStruct(&privateClass{p_secret: 1, Shown: 2}), "Private { shown: 2 }"},
	}
	for _, c := range cases {
		if got := NodeInspect(c.box).ToGoString(); got != c.want {
			t.Errorf("%s renders as %s, want %s", c.name, got, c.want)
		}
	}
}

// TestAClassInstanceRendersInsideAContainer holds the positions where the instance is a
// member rather than the subject: an array element, a map value, a set member.
func TestAClassInstanceRendersInsideAContainer(t *testing.T) {
	box := ObjectFromStruct(newPoint())

	arr := NewArrayValue([]Value{box})
	if got := NodeInspect(arr).ToGoString(); got != "[ Point { x: 1, y: 's' } ]" {
		t.Errorf("an array of one instance renders as %s, want [ Point { x: 1, y: 's' } ]", got)
	}

	m := NewDynMap[Value]()
	m.Set(StringValue(FromGoString("k")), box)
	if got := NodeInspect(m.ToValue()).ToGoString(); got != "Map(1) { 'k' => Point { x: 1, y: 's' } }" {
		t.Errorf("a map holding an instance renders as %s, want Map(1) { 'k' => Point { x: 1, y: 's' } }", got)
	}

	s := NewDynSet()
	s.Add(box)
	if got := NodeInspect(s.ToValue()).ToGoString(); got != "Set(1) { Point { x: 1, y: 's' } }" {
		t.Errorf("a set holding an instance renders as %s, want Set(1) { Point { x: 1, y: 's' } }", got)
	}
}

// TestAClassInstanceAbbreviatesUnderItsName holds the depth cutoff, the one place the
// class name is printed on its own: past the depth limit Node writes [P] rather than the
// [Object] a plain object gets, so the name has to survive the abbreviation too.
func TestAClassInstanceAbbreviatesUnderItsName(t *testing.T) {
	box := ObjectFromStruct(&nestedClass{Inner: newPoint()})

	if got := NodeInspectArgs(box, Undefined, Number(0)).ToGoString(); got != "Nested { inner: [Point] }" {
		t.Errorf("an instance rendered at depth 0 = %s, want Nested { inner: [Point] }", got)
	}
}

// TestAClassInstanceShowsNothingHiddenOfItsOwn holds showHidden, which for an instance
// is the plain rendering: an instance has no internal slot to reveal, the way a typed
// array or a map does, so Node prints exactly what it prints without it.
func TestAClassInstanceShowsNothingHiddenOfItsOwn(t *testing.T) {
	box := ObjectFromStruct(newPoint())

	got := NodeInspectArgs(box, objectOf("showHidden", Bool(true))).ToGoString()
	if got != "Point { x: 1, y: 's' }" {
		t.Errorf("an instance under showHidden = %s, want Point { x: 1, y: 's' }", got)
	}
}

// TestAClassInstanceComparesByItsClass is the second thing the prototype buys. Node's
// strict comparison requires the same constructor, which it decides by prototype
// identity, so two instances of one class can be equal while an instance of another
// class with the same fields, and a plain object with the same fields, cannot.
func TestAClassInstanceComparesByItsClass(t *testing.T) {
	point := ObjectFromStruct(newPoint())
	same := ObjectFromStruct(newPoint())
	other := ObjectFromStruct(&pointClass{X: 2, Y: FromGoString("s")})
	twin := ObjectFromStruct(&twinClass{X: 1, Y: FromGoString("s")})
	plain := NewObject()
	plain.Set(FromGoString("x"), Number(1))
	plain.Set(FromGoString("y"), StringValue(FromGoString("s")))

	if !DeepStrictEqual(point, same) {
		t.Error("two instances of one class with the same fields are not deeply equal")
	}
	if DeepStrictEqual(point, other) {
		t.Error("two instances of one class with different fields are deeply equal")
	}
	if DeepStrictEqual(point, twin) {
		t.Error("instances of two classes with the same fields are strictly deeply equal")
	}
	if DeepStrictEqual(point, plain) {
		t.Error("an instance and a plain object with the same fields are strictly deeply equal")
	}
	// The loose comparison does not read the constructor, so the same three pairs that
	// the strict one separates are equal here, which is what Node's assert.deepEqual does.
	if !DeepEqual(point, twin) {
		t.Error("instances of two classes with the same fields are not loosely deeply equal")
	}
	if !DeepEqual(point, plain) {
		t.Error("an instance and a plain object with the same fields are not loosely deeply equal")
	}
}

// TestAClassInstanceBoxesThroughEveryEntry holds the entries that reach the box without
// naming it: the generic element boxer an array of instances uses, and the reflection
// arm of the JSON walk, which a class field holding another instance goes through.
func TestAClassInstanceBoxesThroughEveryEntry(t *testing.T) {
	arr := ArrayValueOf(NewArray(newPoint(), &pointClass{X: 2}), ClassToValue)
	if got := NodeInspect(arr).ToGoString(); got != "[ Point { x: 1, y: 's' }, Point { x: 2, y: '' } ]" {
		t.Errorf("an array boxed through ClassToValue renders as %s", got)
	}

	// The unboxed entry: a statically typed instance reaches JSON.stringify as the Go
	// struct, so the walk's reflection arm has to produce the same text the boxed one does.
	if got := JSONStringify(newPoint()).ToGoString(); got != `{"x":1,"y":"s"}` {
		t.Errorf("JSON.stringify of an unboxed instance = %s, want {\"x\":1,\"y\":\"s\"}", got)
	}
	if got := JSONStringifyIndentNum(newPoint(), 2).ToGoString(); got != "{\n  \"x\": 1,\n  \"y\": \"s\"\n}" {
		t.Errorf("the indented JSON of an unboxed instance = %q", got)
	}
}

// builtinClass holds a field of each built-in kind that carries a box of its own, the
// shapes the reflection walk used to read as an empty object or as whatever their ToJSON
// answered.
type builtinClass struct {
	D *Date              `json:"d"`
	M *Map[Value, Value] `json:"m"`
	S *Set[Value]        `json:"s"`
	R *RegExp            `json:"r"`
	B *ArrayBuffer       `json:"b"`
}

// TestAClassInstanceBoxesItsBuiltInFields holds the fields whose Go representation is a
// runtime struct rather than a scalar. Each has to take its own box on the way into the
// instance's, since reflecting one reads unexported storage: a date used to print as its
// quoted ISO string, picked up from its ToJSON, and a map as an empty object.
func TestAClassInstanceBoxesItsBuiltInFields(t *testing.T) {
	RegisterClass("Builtin", (*builtinClass)(nil))
	box := ObjectFromStruct(&builtinClass{
		D: NewDateFromMillis(0),
		M: NewDynMap[Value](),
		S: NewDynSet(),
		R: NewRegExpLiteral("ab", "g"),
		B: NewArrayBuffer(4),
	})

	want := "Builtin {\n  d: 1970-01-01T00:00:00.000Z,\n  m: Map(0) {},\n  s: Set(0) {},\n  r: /ab/g,\n" +
		"  b: ArrayBuffer { [Uint8Contents]: <00 00 00 00>, [byteLength]: 4 }\n}"
	if got := NodeInspect(box).ToGoString(); got != want {
		t.Errorf("an instance with built-in fields renders as\n%s\nwant\n%s", got, want)
	}
	// The JSON does not move: the serializer applies each value's toJSON, so the date is
	// still its ISO string, and the rest have no own enumerable key so they stay empty.
	if got := JSONStringify(box).ToGoString(); got != `{"d":"1970-01-01T00:00:00.000Z","m":{},"s":{},"r":{},"b":{}}` {
		t.Errorf("JSON.stringify of an instance with built-in fields = %s", got)
	}
}

// TestAnUnregisteredStructStaysAPlainObject holds the boundary. A fixed-shape object
// struct is not a class and registers nothing, so it boxes with no prototype and prints
// the way Node prints an object literal, with no name in front of the braces.
func TestAnUnregisteredStructStaysAPlainObject(t *testing.T) {
	type shape struct {
		A float64 `json:"a"`
	}
	box := ObjectFromStruct(&shape{A: 1})

	if box.object().proto != nil {
		t.Error("a struct that registered no class was given a prototype")
	}
	if got := NodeInspect(box).ToGoString(); got != "{ a: 1 }" {
		t.Errorf("an unregistered struct renders as %s, want { a: 1 }", got)
	}
}

// TestRegisteringAClassTwiceKeepsTheFirstPrototype holds the interning. The prototype's
// identity is what the deep comparison reads, so a second registration of one Go type
// must not replace it and leave earlier boxes comparing unequal to later ones.
func TestRegisteringAClassTwiceKeepsTheFirstPrototype(t *testing.T) {
	before := ObjectFromStruct(newPoint()).object().proto
	RegisterClass("PointAgain", (*pointClass)(nil))
	after := ObjectFromStruct(newPoint()).object().proto

	if before != after {
		t.Error("registering a class twice replaced its prototype")
	}
	if got := NodeInspect(ObjectFromStruct(newPoint())).ToGoString(); got != "Point { x: 1, y: 's' }" {
		t.Errorf("after a second registration an instance renders as %s", got)
	}
}
