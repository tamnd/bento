package value

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// util.format is what console.log('%s: %d', name, n) prints, so these hold the port
// to real Node output rather than to a reading of the docs. The specifier rules are
// full of decisions no one would guess (a specifier past the last argument stays
// literal, %% collapses only when something else in the string substituted, %c
// consumes an argument and prints nothing), and every one of them is a case below.
//
// testdata/nodeformat_node24.json was produced by testdata/gen_nodeformat_ref.js
// under Node v24.18.0, and testdata/nodeinspectopts_node24.json by
// gen_nodeinspectopts_ref.js. Cases are matched by name in both directions, so a
// case on one side and not the other fails rather than being skipped.

// formatCase is one call: the arguments, and the options when the case goes through
// util.formatWithOptions rather than util.format.
type formatCase struct {
	args    []Value
	options Value
	hasOpts bool
}

// formatCases builds each named call. The names match the generator's keys.
func formatCases(t *testing.T) map[string]formatCase {
	t.Helper()

	str := func(s string) Value { return StringValue(FromGoString(s)) }
	obj := func(pairs ...any) Value {
		o := NewObject()
		for i := 0; i < len(pairs); i += 2 {
			o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
		}
		return o
	}
	arr := func(elems ...Value) Value { return NewArrayValue(elems) }
	bigOf := func(s string) Value {
		v, ok := BigIntFromString(s)
		if !ok {
			t.Fatalf("cannot parse bigint %q", s)
		}
		return v
	}
	fmtc := func(args ...Value) formatCase { return formatCase{args: args} }
	optc := func(options Value, args ...Value) formatCase {
		return formatCase{args: args, options: options, hasOpts: true}
	}

	circular := obj("a", Number(1))
	circular.Set(FromGoString("self"), circular)

	ownToString := NewObject()
	ownToString.Set(FromGoString("toString"), NewFunc(func([]Value) Value { return str("own") }))

	ownToPrimitive := NewObject()
	ownToPrimitive.SetElem(SymbolToPrimitive(), NewFunc(func([]Value) Value { return str("prim") }))

	return map[string]formatCase{
		"no arguments":                fmtc(),
		"one string":                  fmtc(str("hello")),
		"one string with escape":      fmtc(str("a%%b")),
		"one string with specifier":   fmtc(str("%s")),
		"first argument not a string": fmtc(obj("a", Number(1)), str("b")),
		"first argument a number":     fmtc(Number(1), Number(2)),

		"s string":           fmtc(str("%s"), str("a")),
		"s number":           fmtc(str("%s"), Number(42)),
		"s negative zero":    fmtc(str("%s"), Number(negZero())),
		"s fraction":         fmtc(str("%s"), Number(-1234567.891)),
		"s nan":              fmtc(str("%s"), Number(nan())),
		"s bigint":           fmtc(str("%s"), bigOf("10")),
		"s boolean":          fmtc(str("%s"), True),
		"s null":             fmtc(str("%s"), Null),
		"s undefined":        fmtc(str("%s"), Undefined),
		"s symbol":           fmtc(str("%s"), NewSymbol(FromGoString("s"))),
		"s object":           fmtc(str("%s"), obj("a", Number(1))),
		"s nested object":    fmtc(str("%s"), obj("a", obj("b", obj("c", Number(1))))),
		"s array":            fmtc(str("%s"), arr(Number(1), Number(2))),
		"s nested array":     fmtc(str("%s"), arr(arr(Number(1)), arr(Number(2)))),
		"s own toString":     fmtc(str("%s"), ownToString),
		"s own toPrimitive":  fmtc(str("%s"), ownToPrimitive),
		"s null prototype":   fmtc(str("%s"), ObjectCreate(Null)),
		"s regexp":           fmtc(str("%s"), RegExpValue(NewRegExpLiteral("ab+c", "gi"))),
		"s error":            fmtc(str("%s"), NewTypeError(FromGoString("bad")).ToValue()),
		"s two":              fmtc(str("%s %s"), str("a"), str("b")),
		"s missing argument": fmtc(str("%s %s"), str("a")),
		"s then text":        fmtc(str("%s: done"), str("a")),
		"s leading text":     fmtc(str("value is %s"), str("a")),

		"d integer":       fmtc(str("%d"), Number(42)),
		"d negative zero": fmtc(str("%d"), Number(negZero())),
		"d string":        fmtc(str("%d"), str("10")),
		"d hex string":    fmtc(str("%d"), str("0x10")),
		"d not a number":  fmtc(str("%d"), str("abc")),
		"d bigint":        fmtc(str("%d"), bigOf("10")),
		"d symbol":        fmtc(str("%d"), NewSymbol(FromGoString("s"))),
		"d null":          fmtc(str("%d"), Null),
		"d undefined":     fmtc(str("%d"), Undefined),
		"d object":        fmtc(str("%d"), obj("a", Number(1))),
		"d array of one":  fmtc(str("%d"), arr(Number(5))),
		"i trailing text": fmtc(str("%i"), str("42.9px")),
		"i hex string":    fmtc(str("%i"), str("0x10")),
		"i fraction":      fmtc(str("%i"), Number(42.9)),
		"i bigint":        fmtc(str("%i"), bigOf("10")),
		"i symbol":        fmtc(str("%i"), NewSymbol(FromGoString("s"))),
		"i not a number":  fmtc(str("%i"), str("abc")),
		"f trailing text": fmtc(str("%f"), str("3.5abc")),
		"f leading dot":   fmtc(str("%f"), str(".5")),
		"f bigint":        fmtc(str("%f"), bigOf("10")),
		"f symbol":        fmtc(str("%f"), NewSymbol(FromGoString("s"))),
		"f not a number":  fmtc(str("%f"), str("abc")),

		"j object":    fmtc(str("%j"), obj("a", Number(1))),
		"j nested":    fmtc(str("%j"), arr(Number(1), obj("a", Number(2)))),
		"j string":    fmtc(str("%j"), str("str")),
		"j number":    fmtc(str("%j"), Number(1.5)),
		"j undefined": fmtc(str("%j"), Undefined),
		"j function":  fmtc(str("%j"), WithName(NewFunc(func([]Value) Value { return Undefined }), "foo")),
		"j symbol":    fmtc(str("%j"), NewSymbol(FromGoString("s"))),
		"j circular":  fmtc(str("%j"), circular),

		"o array":  fmtc(str("%o"), arr(Number(1), Number(2), Number(3))),
		"o object": fmtc(str("%o"), obj("a", Number(1))),
		"o deep":   fmtc(str("%o"), obj("a", obj("b", obj("c", obj("d", obj("e", Number(1))))))),
		"O object": fmtc(str("%O"), obj("a", Number(1))),
		"O deep":   fmtc(str("%O"), obj("a", obj("b", obj("c", obj("d", Number(1)))))),
		"O string": fmtc(str("%O"), str("a")),

		"c consumes an argument":      fmtc(str("%c"), str("css"), str("rest")),
		"c inline":                    fmtc(str("a%cb"), str("css")),
		"percent with argument":       fmtc(str("%%"), str("x")),
		"percent before specifier":    fmtc(str("%% %s"), str("x")),
		"percent glued to specifier":  fmtc(str("%%%s"), str("x")),
		"unknown specifier":           fmtc(str("%z"), Number(1)),
		"trailing percent":            fmtc(str("100%"), str("x")),
		"lone percent":                fmtc(str("%"), str("x")),
		"specifier then percent":      fmtc(str("%s%"), str("x")),
		"leftovers of every kind":     fmtc(str("%s"), str("a"), str("b"), Number(1), obj("x", Number(1))),
		"leftover null and undefined": fmtc(str("%s"), Null, Undefined, True),
		"leftover with no specifier":  fmtc(str("plain"), str("a"), Number(1)),

		"options numeric separator d":      optc(obj("numericSeparator", True), str("%d"), Number(1234567)),
		"options numeric separator s":      optc(obj("numericSeparator", True), str("%s"), Number(-1234567.891)),
		"options numeric separator i":      optc(obj("numericSeparator", True), str("%i"), str("1234567")),
		"options numeric separator bigint": optc(obj("numericSeparator", True), str("%d"), bigOf("1234567")),
		"options depth zero":               optc(obj("depth", Number(0)), str("%O"), obj("a", obj("b", Number(1)))),
		"options depth one":                optc(obj("depth", Number(1)), str("%O"), obj("a", obj("b", obj("c", Number(1))))),
		"options empty":                    optc(NewObject(), str("%s"), obj("a", Number(1))),
		"options array":                    optc(NewArrayValue(nil), str("%s"), obj("a", Number(1))),
		"options break length":             optc(obj("breakLength", Number(10)), str("%O"), obj("aaa", Number(1), "bbb", Number(2), "ccc", Number(3))),
		"options sorted":                   optc(obj("sorted", True), str("%O"), obj("c", Number(1), "a", Number(2), "b", Number(3))),
		"options max array length":         optc(obj("maxArrayLength", Number(2)), str("%O"), arr(Number(1), Number(2), Number(3), Number(4))),
		"options leftover":                 optc(obj("depth", Number(0)), str("leftover"), obj("a", obj("b", Number(1)))),
	}
}

// TestFormatMatchesNode is the point of the port: every call renders byte for byte
// the way Node v24.18.0 rendered it.
func TestFormatMatchesNode(t *testing.T) {
	want := readReference(t, "testdata/nodeformat_node24.json")
	got := formatCases(t)

	for name, expected := range want {
		c, ok := got[name]
		if !ok {
			t.Errorf("the reference has case %q but the Go side does not build it", name)
			continue
		}
		actual := NodeFormat(c.args...).ToGoString()
		if c.hasOpts {
			actual = NodeFormatWithOptions(c.options, c.args...).ToGoString()
		}
		if actual != expected {
			t.Errorf("case %q:\n got %q\nwant %q", name, actual, expected)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("the Go side builds case %q but the reference has no entry; regenerate testdata", name)
		}
	}
}

// inspectOptionCases builds the argument list for each named util.inspect call, the
// value first and then whatever options the case is about. The names match
// gen_nodeinspectopts_ref.js.
func inspectOptionCases(t *testing.T) map[string][]Value {
	t.Helper()

	str := func(s string) Value { return StringValue(FromGoString(s)) }
	obj := func(pairs ...any) Value {
		o := NewObject()
		for i := 0; i < len(pairs); i += 2 {
			o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
		}
		return o
	}
	arr := func(elems ...Value) Value { return NewArrayValue(elems) }
	bigOf := func(s string) Value {
		v, ok := BigIntFromString(s)
		if !ok {
			t.Fatalf("cannot parse bigint %q", s)
		}
		return v
	}
	nums := func(n int) Value {
		elems := make([]Value, n)
		for i := range elems {
			elems[i] = Number(float64(i))
		}
		return NewArrayValue(elems)
	}
	getter := func(f func() Value) Value {
		return NewFunc(func([]Value) Value { return f() })
	}

	hiddenProp := obj("a", Number(1))
	hiddenProp.object().defineOwn(FromGoString("hidden"), dataProperty(Number(2), false, false, false))

	hiddenSymbol := obj("a", Number(1))
	hiddenSymbol.object().defineSym(NewSymbol(FromGoString("s")).symbol(),
		dataProperty(Number(2), false, false, false))

	accessors := obj("plain", Number(1))
	setter := NewFunc(func([]Value) Value { return Undefined })
	accessors.object().defineOwn(FromGoString("onlyGet"),
		accessorProperty(getter(func() Value { return Number(5) }), Undefined, true, true))
	accessors.object().defineOwn(FromGoString("onlySet"),
		accessorProperty(Undefined, setter, true, true))
	accessors.object().defineOwn(FromGoString("both"),
		accessorProperty(getter(func() Value { return Number(6) }), setter, true, true))
	accessors.object().defineOwn(FromGoString("objectGet"),
		accessorProperty(getter(func() Value { return obj("deep", Number(1)) }), Undefined, true, true))

	deep := obj("a", obj("b", obj("c", obj("d", obj("e", Number(1))))))
	wide := obj("aaa", Number(1), "bbb", Number(2), "ccc", Number(3))
	longString := str(strings.Repeat("x", 20))
	// The handler carries a get trap named the way the generator's does, since
	// showProxy prints the handler and a named function reads as its name.
	proxied := NewProxy(obj("a", Number(1)),
		obj("get", WithName(NewFunc(func(args []Value) Value { return Arg(args, 0).GetElem(Arg(args, 1)) }), "get")))

	return map[string][]Value{
		"show hidden array":                       {arr(Number(1), Number(2), Number(3)), obj("showHidden", True)},
		"show hidden array of one":                {arr(str("a")), obj("showHidden", True)},
		"show hidden non enumerable":              {hiddenProp, obj("showHidden", True)},
		"non enumerable hidden by default":        {hiddenProp, NewObject()},
		"show hidden symbol":                      {hiddenSymbol, obj("showHidden", True)},
		"non enumerable symbol hidden by default": {hiddenSymbol, NewObject()},
		"show hidden nested array":                {obj("a", arr(Number(1))), obj("showHidden", True)},
		"show hidden as second positional":        {arr(Number(1), Number(2)), True},

		"sorted":                   {obj("c", Number(1), "a", Number(2), "b", Number(3)), obj("sorted", True)},
		"sorted array keeps order": {arr(Number(3), Number(1), Number(2)), obj("sorted", True)},
		"sorted nested":            {obj("z", obj("y", Number(1), "x", Number(2))), obj("sorted", True)},
		"sorted with hidden keys":  {hiddenProp, obj("sorted", True, "showHidden", True)},
		"sorted comparator": {obj("a", Number(1), "b", Number(2), "c", Number(3)),
			obj("sorted", NewFunc(func(args []Value) Value {
				if ToString(Arg(args, 0)).Compare(ToString(Arg(args, 1))) < 0 {
					return Number(1)
				}
				return Number(-1)
			}))},

		"getters off": {accessors, NewObject()},
		"getters all": {accessors, obj("getters", True)},
		"getters get": {accessors, obj("getters", str("get"))},
		"getters set": {accessors, obj("getters", str("set"))},

		"numeric separator integer":  {Number(1234567), obj("numericSeparator", True)},
		"numeric separator negative": {Number(-1234567), obj("numericSeparator", True)},
		"numeric separator fraction": {Number(-1234567.891), obj("numericSeparator", True)},
		"numeric separator small":    {Number(123), obj("numericSeparator", True)},
		"numeric separator exponent": {Number(1e21), obj("numericSeparator", True)},
		"numeric separator infinity": {Number(inf(1)), obj("numericSeparator", True)},
		"numeric separator nan":      {Number(nan()), obj("numericSeparator", True)},
		"numeric separator bigint":   {bigOf("1234567"), obj("numericSeparator", True)},
		"numeric separator in array": {arr(Number(1234567)), obj("numericSeparator", True)},

		"compact false":        {wide, obj("compact", False)},
		"compact false nested": {obj("a", obj("b", Number(1))), obj("compact", False)},
		"compact false array":  {nums(8), obj("compact", False)},
		"compact true":         {wide, obj("compact", True)},
		"compact true nested":  {deep, obj("compact", True)},
		"compact true long":    {obj("a", longString, "b", longString), obj("compact", True)},
		"compact one":          {wide, obj("compact", Number(1))},
		"compact one deep":     {obj("a", obj("b", obj("c", Number(1)))), obj("compact", Number(1))},

		"depth null":     {deep, obj("depth", Null)},
		"depth three":    {deep, obj("depth", Number(3))},
		"depth negative": {obj("a", Number(1)), obj("depth", Number(-1))},
		"depth infinity": {deep, obj("depth", Number(inf(1)))},

		"max array length zero":       {arr(Number(1), Number(2), Number(3)), obj("maxArrayLength", Number(0))},
		"max array length one":        {arr(Number(1), Number(2), Number(3)), obj("maxArrayLength", Number(1))},
		"max array length null":       {nums(8), obj("maxArrayLength", Null)},
		"max string length":           {longString, obj("maxStringLength", Number(5))},
		"max string length zero":      {longString, obj("maxStringLength", Number(0))},
		"max string length in object": {obj("s", longString), obj("maxStringLength", Number(5))},

		"break length small": {wide, obj("breakLength", Number(10))},
		"break length large": {nums(8), obj("breakLength", Number(1000))},
		"break length one":   {obj("a", Number(1)), obj("breakLength", Number(1))},

		"proxy hidden by default": {proxied, NewObject()},
		"show proxy":              {proxied, obj("showProxy", True)},
		"show proxy nested":       {obj("p", proxied), obj("showProxy", True)},

		"depth as positional": {deep, Undefined, Number(0)},
	}
}

// TestInspectOptionsMatchNode covers the option surface util.format needs: %s
// inspects at depth 0, %o with showHidden and showProxy, and formatWithOptions
// hands the caller's options straight through. Every one of those is an option that
// had to be built before the format specifiers could be exact, so each is pinned
// against Node directly rather than only through a format string.
func TestInspectOptionsMatchNode(t *testing.T) {
	want := readReference(t, "testdata/nodeinspectopts_node24.json")
	got := inspectOptionCases(t)

	for name, expected := range want {
		args, ok := got[name]
		if !ok {
			t.Errorf("the reference has case %q but the Go side does not build it", name)
			continue
		}
		if actual := NodeInspectArgs(args...).ToGoString(); actual != expected {
			t.Errorf("case %q:\n got %q\nwant %q", name, actual, expected)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("the Go side builds case %q but the reference has no entry; regenerate testdata", name)
		}
	}
}

// readReference loads one of the Node-generated reference files.
func readReference(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the reference: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parsing the reference: %v", err)
	}
	return out
}

// TestAGetterThatThrewIsReported is the case the generated reference cannot hold,
// because Node inspects the thrown error there and a real error's stack names the
// file it was thrown in, so the case would record where it was generated. Node with
// the stack flattened the way a bento error carries it prints exactly the string
// below, checked directly against node v24.18.0. What matters is that the throw is
// reported in place rather than escaping and killing the log line.
func TestAGetterThatThrewIsReported(t *testing.T) {
	o := NewObject()
	o.object().defineOwn(FromGoString("bad"), accessorProperty(
		NewFunc(func([]Value) Value {
			Throw(NewError(FromGoString("nope")))
			return Undefined
		}), Undefined, true, true))
	got := NodeInspectArgs(o, mustObj("getters", True)).ToGoString()
	want := "{ bad: [Getter: <Inspection threw ([Error: nope])>] }"
	if got != want {
		t.Errorf("inspect of a throwing getter = %q, want %q", got, want)
	}
}

// TestColorsIsRefusedRatherThanIgnored pins the one option this slice does not
// implement. Coloring means threading a stylize function through every token the
// inspector emits, which is its own change; accepting the option and printing
// uncolored output would leave a program silently missing the escapes it asked for,
// so the option throws instead.
func TestColorsIsRefusedRatherThanIgnored(t *testing.T) {
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("inspect with colors did not throw")
		}
		if msg := Caught(rec.(Thrown)).ErrorMessage(); !strings.Contains(msg, "colors") {
			t.Errorf("thrown message is %q, want it to mention colors", msg)
		}
	}()
	NodeInspectArgs(Number(1), mustObj("colors", True))
}

// TestColorsFalseIsAccepted is the other half: false is the default, so a caller
// passing it explicitly (which console does) is asking for nothing and gets it.
func TestColorsFalseIsAccepted(t *testing.T) {
	if got := NodeInspectArgs(Number(1), mustObj("colors", False)).ToGoString(); got != "1" {
		t.Errorf("inspect with colors: false = %q, want 1", got)
	}
}

// TestFormatWithOptionsRefusesANonObject pins Node's validation. A program that
// passes its format string where the options go should learn so from
// ERR_INVALID_ARG_TYPE rather than have the string silently read as options.
func TestFormatWithOptionsRefusesANonObject(t *testing.T) {
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("formatWithOptions with a number for options did not throw")
		}
		e := Caught(rec.(Thrown))
		code, has := e.Code()
		if !has || code.ToGoString() != "ERR_INVALID_ARG_TYPE" {
			t.Errorf("thrown code is %q (present %v), want ERR_INVALID_ARG_TYPE", code.ToGoString(), has)
		}
		want := `The "inspectOptions" argument must be of type object. Received type number (1)`
		if got := e.ErrorMessage(); got != want {
			t.Errorf("thrown message is %q, want %q", got, want)
		}
	}()
	NodeFormatWithOptions(Number(1), StringValue(FromGoString("x")))
}

// TestTheCircularMessageIsWhatJSONThrows ties %j's [Circular] to the error it
// recognizes. The two are in different files, and if JSON.stringify's wording ever
// moved, %j would start re-throwing on a cycle rather than printing [Circular]; this
// fails first.
func TestTheCircularMessageIsWhatJSONThrows(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("self"), o)
	var thrown *Error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				thrown = Caught(rec.(Thrown))
			}
		}()
		var b strings.Builder
		encodeBoxedJSON(&b, o, nil)
	}()
	if thrown == nil {
		t.Fatal("stringifying a cycle did not throw")
	}
	if thrown.ErrorName() != "TypeError" {
		t.Errorf("thrown name is %q, want TypeError", thrown.ErrorName())
	}
	if got := firstErrorLine(thrown); got != circularErrorMessage {
		t.Errorf("first line of the message is %q, want %q", got, circularErrorMessage)
	}
}

// TestConsoleFormatIsOneLine pins what the lowerer emits: console.log hands its
// arguments to one call that answers the whole line, so the stream writes it in one
// piece rather than joining parts with spaces of its own.
func TestConsoleFormatIsOneLine(t *testing.T) {
	args := []Value{
		StringValue(FromGoString("%s is %d")),
		StringValue(FromGoString("x")),
		Number(2),
	}
	if got := ConsoleFormat(args...).ToGoString(); got != "x is 2" {
		t.Errorf("ConsoleFormat = %q, want x is 2", got)
	}
}

// TestAFunctionUnderPercentSIsNamed records a divergence bento cannot close. Node
// prints the function's source text, which an AOT binary does not carry; the inspect
// form names the function instead, which is the useful half of what Node prints.
func TestAFunctionUnderPercentSIsNamed(t *testing.T) {
	f := WithName(NewFunc(func([]Value) Value { return Undefined }), "foo")
	got := NodeFormat(StringValue(FromGoString("%s")), f).ToGoString()
	if got != "[Function: foo]" {
		t.Errorf("format(\"%%s\", foo) = %q, want [Function: foo]", got)
	}
}

// mustObj builds a small options object, the shape every option test passes.
func mustObj(pairs ...any) Value {
	o := NewObject()
	for i := 0; i < len(pairs); i += 2 {
		o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
	}
	return o
}
