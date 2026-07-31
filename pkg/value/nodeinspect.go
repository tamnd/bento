// This file is a port of Node's util.inspect (lib/internal/util/inspect.js), the
// renderer behind console.log. Every Node program logs an object, so the exact
// spelling of that output is one of the most visible pieces of Node's surface
// there is: "{ a: 1, b: 'x' }" with the spaces inside the braces, single quotes on
// strings unless the string holds one, "[Object]" once past two levels deep,
// "[Circular *1]" on a cycle, and the column grouping a long numeric array gets.
// Programs are read against that output and tests are written against it, so an
// approximation is not compatibility.
//
// bento's own compact inspector lives in inspect.go and stays: it is what the
// interpreter's __bento_inspect callee answers, and the two paths agree with each
// other rather than with Node. This one is what a compiled program's console.log
// reaches, so it follows Node statement for statement, including the arithmetic
// that decides how many columns a grouped array gets.
//
// The port covers the value kinds bento can box: the primitives, plain objects,
// arrays, functions, regexps, errors, proxies, and the two collections, a Map and a
// Set. Date, the typed arrays and the rest are concrete Go types that do not box into
// a Value yet, so their branches in Node's formatRaw have no counterpart here and are
// named in the implementation note rather than half-written.

package value

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unsafe"
)

// The defaults util.inspect.defaultOptions reports, which are also the options
// console.log passes. Every one of them shapes the output, so they are named here
// rather than spelled inline: depth decides when a nested object becomes
// "[Object]", breakLength and compact together decide when the output wraps onto
// several lines, and the two maxima decide where a long array or string is cut.
const (
	inspectDepth           = 2
	inspectBreakLength     = 80
	inspectCompact         = 3
	inspectMaxArrayLength  = 100
	inspectMaxStringLength = 10000
)

// kMinLineLength is Node's floor on splitting a long string across lines: below it
// a string stays on one line however narrow the remaining width is, so a deeply
// indented short string does not shatter into one-character fragments.
const kMinLineLength = 16

// The three property-formatting modes Node distinguishes. An object property
// renders as "name: value"; an array element renders as the bare value; an array's
// extra named properties render as "name: value" but group with the elements, so
// they need their own tag to keep the element and the property spelling apart
// while still taking the array's line-grouping.
const (
	kObjectType = iota
	kArrayType
	kArrayExtrasType
)

// The three spellings compact takes. A number is the default and the interesting
// one: it caps how many levels deep a subtree may be and still collapse onto one
// line. false turns that off, so every entry gets its own line. true is the oldest
// form, which lays entries out on one line whenever they fit and indents
// differently when they do not, and it is a separate layout rather than a limit,
// which is why it is a mode here rather than a number.
const (
	compactNumber = iota
	compactFalse
	compactTrue
)

// The three spellings getters takes: off, on for every accessor, or on for one
// side of the pair only. Reading a getter runs the program's own code, which is
// why it is off by default and why the mode has to travel with the options.
const (
	gettersOff = iota
	gettersAll
	gettersGet
	gettersSet
)

// inspectCtx is Node's ctx object: the options plus the mutable bookkeeping the
// recursion threads through itself. seen is the stack of references currently
// being formatted, which is how a cycle is caught; circular numbers the references
// a cycle pointed back at, so the "<ref *1>" marker and the "[Circular *1]" that
// refers to it agree. currentDepth is the depth the deepest nested formatRaw
// reached, which is what decides whether a level may collapse onto one line, and
// it is deliberately not restored on the way back up, matching Node.
type inspectCtx struct {
	inspectOptions
	indentationLvl int
	seen           []unsafe.Pointer
	currentDepth   int
	circular       map[unsafe.Pointer]int
}

// inspectOptions is the option half of Node's ctx, split out because util.format
// re-inspects an argument with a few options changed and the rest carried over:
// %s asks for depth 0 and %o for a hidden-property view, both starting from
// whatever the caller passed. Keeping the options in their own value makes that a
// copy with two fields written rather than a field-by-field rebuild.
type inspectOptions struct {
	depth            int
	breakLength      int
	compact          int
	compactKind      int
	maxArrayLength   int
	maxStringLength  int
	showHidden       bool
	showProxy        bool
	numericSeparator bool
	sorted           bool
	sortComparator   Value
	getters          int
}

// defaultInspectOptions is what util.inspect.defaultOptions reports, which is also
// what console.log passes.
func defaultInspectOptions() inspectOptions {
	return inspectOptions{
		depth:           inspectDepth,
		breakLength:     inspectBreakLength,
		compact:         inspectCompact,
		maxArrayLength:  inspectMaxArrayLength,
		maxStringLength: inspectMaxStringLength,
	}
}

// newInspectCtx starts a fresh recursion over the default options.
func newInspectCtx() *inspectCtx {
	return &inspectCtx{inspectOptions: defaultInspectOptions()}
}

// inspectWith renders a value under a given set of options, the one entry point
// util.format's specifiers reach: each of them inspects with its own options and
// needs its own recursion state, which is what makes this a new context every time
// rather than a reused one.
func inspectWith(o inspectOptions, v Value) string {
	c := &inspectCtx{inspectOptions: o}
	return c.formatValue(v, 0)
}

// NodeInspect renders a value the way Node's util.inspect does with its default
// options, which is what console.log prints for every argument that is not already
// a string. It is the console's renderer rather than a string coercion: an object
// reads as its properties instead of "[object Object]", a string nested in a
// container is quoted, and a bigint carries its "n".
func NodeInspect(v Value) BStr {
	return FromGoString(newInspectCtx().formatValue(v, 0))
}

// NodeInspectArgs is util.inspect called with its own argument list, so the module
// entry point and this port read the options in one place. Node still accepts the
// positional form inspect(value, showHidden, depth, colors) it started with, and
// reads it before the options object, so both are handled here in that order. It is
// variadic because the lowerer emits a call to it for an imported node:util inspect,
// one boxed argument per source argument, and a variadic signature is what an emitted
// call can name without building a slice literal.
func NodeInspectArgs(args ...Value) BStr {
	o := defaultInspectOptions()
	if len(args) > 2 {
		if args[2].kind != KindUndefined {
			o.setDepth(args[2])
		}
		if len(args) > 3 && args[3].kind != KindUndefined {
			o.setColors(args[3])
		}
	}
	if len(args) > 1 {
		switch opts := args[1]; {
		case opts.kind == KindBool:
			o.showHidden = opts.AsBool()
		case ToBoolean(opts):
			o.readOptions(opts)
		}
	}
	return FromGoString(inspectWith(o, Arg(args, 0)))
}

// readOptions copies an options object onto the options, the loop inspect runs over
// ObjectKeys(opts). A key outside the option set is kept by Node only to pass on to
// a value's own inspect function, which bento does not call yet, so it is ignored
// here rather than rejected: an unknown key changes nothing about the output.
func (o *inspectOptions) readOptions(opts Value) {
	for _, k := range opts.OwnEnumerableKeys().Elems() {
		v := opts.Get(k)
		switch k.ToGoString() {
		case "showHidden":
			o.showHidden = ToBoolean(v)
		case "depth":
			o.setDepth(v)
		case "colors":
			o.setColors(v)
		case "showProxy":
			o.showProxy = ToBoolean(v)
		case "maxArrayLength":
			o.maxArrayLength = inspectLimit(v)
		case "maxStringLength":
			o.maxStringLength = inspectLimit(v)
		case "breakLength":
			o.breakLength = inspectLimit(v)
		case "compact":
			o.setCompact(v)
		case "sorted":
			o.setSorted(v)
		case "getters":
			o.setGetters(v)
		case "numericSeparator":
			o.numericSeparator = ToBoolean(v)
		case "customInspect":
			// Accepted and ignored. The default is on, so rejecting it would reject the
			// option every caller that spells the defaults out passes, and bento does not
			// call a value's own inspect function yet either way, so on and off agree
			// until it does.
		}
	}
}

// setDepth reads the depth option, which is a count of levels or null for no limit
// at all. Node spells the limit as null and tests for it everywhere it compares a
// depth; the same effect here is a limit no recursion can reach, so the comparisons
// stay plain.
func (o *inspectOptions) setDepth(v Value) {
	if v.kind == KindNull {
		o.depth = math.MaxInt
		return
	}
	o.depth = inspectLimit(v)
}

// setColors rejects the one option this port cannot honor. Coloring is a style
// function threaded through every token the formatter emits, which is a change to
// every line of it rather than a flag, so a caller asking for colors is told that
// rather than handed uncolored text it did not ask for.
func (o *inspectOptions) setColors(v Value) {
	if ToBoolean(v) {
		Throw(NewError(FromGoString("util.inspect with colors is not implemented in bento yet")))
	}
}

// setCompact reads the compact option in its three spellings, a number, false, or
// true, each of which is a different layout rather than a different limit.
func (o *inspectOptions) setCompact(v Value) {
	switch v.kind {
	case KindBool:
		if v.AsBool() {
			o.compactKind = compactTrue
		} else {
			o.compactKind = compactFalse
		}
	default:
		o.compactKind = compactNumber
		o.compact = inspectLimit(v)
	}
}

// setSorted reads the sorted option: true sorts the entries the way the language's
// own sort does, and a function sorts them through that comparator, which is called
// with the two already-rendered entries.
func (o *inspectOptions) setSorted(v Value) {
	switch v.kind {
	case KindFunc:
		o.sorted = true
		o.sortComparator = v
	default:
		o.sorted = ToBoolean(v)
		o.sortComparator = Value{}
	}
}

// setGetters reads the getters option, which decides whether an accessor is read
// or only reported. "get" and "set" name one side of the pair, so an object whose
// getter is cheap can be shown while its setter's partner is left alone.
func (o *inspectOptions) setGetters(v Value) {
	if v.kind == KindString {
		switch v.str().ToGoString() {
		case "get":
			o.getters = gettersGet
			return
		case "set":
			o.getters = gettersSet
			return
		}
	}
	if ToBoolean(v) {
		o.getters = gettersAll
		return
	}
	o.getters = gettersOff
}

// inspectLimit reads one of the numeric options. Node spells "no limit" as null and
// accepts Infinity for the same thing, and both become the largest int here so the
// comparison that uses it can stay a plain one.
func inspectLimit(v Value) int {
	if v.kind == KindNull {
		return math.MaxInt
	}
	f := ToNumber(v)
	switch {
	case f != f:
		return 0
	case f >= float64(math.MaxInt):
		return math.MaxInt
	case f <= float64(math.MinInt):
		return math.MinInt
	}
	return int(f)
}

// formatValue is Node's formatValue: primitives render directly, null is its own
// word, a proxy is replaced by its target (showProxy is off by default, so Node
// inspects what the proxy stands for rather than the trap pair), and a reference
// already on the stack is a cycle. Everything else goes to formatRaw.
func (c *inspectCtx) formatValue(v Value, recurseTimes int) string {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	case KindNull:
		return "null"
	default:
		return c.formatPrimitive(v)
	}

	// A proxy stands in for its target. The loop rather than a single step covers a
	// proxy whose target is itself a proxy, which Node reaches by re-entering
	// getProxyDetails on the replaced value.
	for {
		p := v.object().proxy
		if p == nil {
			break
		}
		if p.revoked {
			return "<Revoked Proxy>"
		}
		if c.showProxy {
			return c.formatProxy(p, recurseTimes)
		}
		v = p.target
		switch v.kind {
		case KindObject, KindArray, KindFunc:
		default:
			return c.formatValue(v, recurseTimes)
		}
	}

	for _, seen := range c.seen {
		if seen == v.ref {
			if c.circular == nil {
				c.circular = map[unsafe.Pointer]int{}
			}
			index, ok := c.circular[v.ref]
			if !ok {
				index = len(c.circular) + 1
				c.circular[v.ref] = index
			}
			return "[Circular *" + strconv.Itoa(index) + "]"
		}
	}
	return c.formatRaw(v, recurseTimes)
}

// sortOutput orders already-rendered entries in place, the sorted option. With no
// comparator it is the language's own rule, the entries compared as strings by code
// unit, which is what Array.prototype.sort does with no argument; a comparator is
// called with the two entries the way sort calls one.
func (c *inspectCtx) sortOutput(output []string) {
	if c.sortComparator.kind == KindFunc {
		sort.SliceStable(output, func(i, j int) bool {
			a := StringValue(FromGoString(output[i]))
			b := StringValue(FromGoString(output[j]))
			return ToNumber(c.sortComparator.Call(a, b)) < 0
		})
		return
	}
	sort.SliceStable(output, func(i, j int) bool {
		return FromGoString(output[i]).Compare(FromGoString(output[j])) < 0
	})
}

// formatProxy is Node's formatProxy, what showProxy asks for: the target and the
// handler side by side rather than the object the proxy stands for. It is the only
// view that shows a proxy is there at all, which is why a program debugging one
// turns the option on, and it is what the %o specifier passes.
func (c *inspectCtx) formatProxy(p *proxyData, recurseTimes int) string {
	if recurseTimes > c.depth {
		return "Proxy [Array]"
	}
	recurseTimes++
	c.indentationLvl += 2
	output := []string{
		c.formatValue(p.target, recurseTimes),
		c.formatValue(p.handler, recurseTimes),
	}
	c.indentationLvl -= 2
	return c.reduceToSingleString(output, "", [2]string{"Proxy [", "]"}, kArrayExtrasType, recurseTimes, Undefined)
}

// inspectKey is one entry of the key list a formatted object walks. A string key
// and a symbol key render differently and are stored differently, so the two ride
// one type rather than two parallel lists that could fall out of order.
type inspectKey struct {
	str BStr
	sym *Symbol
}

// formatRaw is Node's formatRaw: it picks the braces, the base text that precedes
// them, and the formatter that produces the entries, then hands all three to
// reduceToSingleString to be laid out. The early returns are the shapes that have
// no entries at all and so never reach the layout: an empty array, a bare
// function, a regexp with no extra properties.
func (c *inspectCtx) formatRaw(v Value, recurseTimes int) string {
	o := v.object()
	constructor, ctorNull := inspectConstructorName(v)

	base := ""
	braces := [2]string{"{", "}"}
	extrasType := kObjectType
	var keys []inspectKey
	var formatter func() []string

	switch {
	case v.kind == KindArray:
		prefix := ""
		if constructor != "Array" || ctorNull {
			prefix = inspectPrefix(constructor, ctorNull, "Array", "("+strconv.Itoa(len(o.elems))+")")
		}
		keys = inspectNonIndexKeys(o, c.showHidden)
		braces = [2]string{prefix + "[", "]"}
		if len(o.elems) == 0 && len(keys) == 0 {
			return braces[0] + "]"
		}
		extrasType = kArrayExtrasType
		// The closure reads recurseTimes at call time, which is after the increment
		// below, so the elements are formatted one level deeper than this value: the
		// same depth formatProperty gets handed for a key.
		formatter = func() []string { return c.formatArray(v, recurseTimes) }

	case v.kind == KindFunc:
		keys = inspectObjectKeys(o, c.showHidden)
		base = inspectFunctionBase(v, constructor, ctorNull)
		if len(keys) == 0 {
			return base
		}

	case v.asRegExp() != nil:
		keys = inspectObjectKeys(o, c.showHidden)
		base = v.asRegExp().ToStringBStr().ToGoString()
		if len(keys) == 0 || recurseTimes > c.depth {
			return base
		}

	case o.jsMap != nil:
		// A Map renders its entries rather than its properties, since it has none: the
		// size rides the prefix and each entry reads "key => value". An empty map with no
		// properties of its own is the whole output, "Map(0) {}", so it returns here
		// before the depth handling the way an empty array does.
		prefix := inspectPrefix(constructor, ctorNull, "Map", "("+strconv.Itoa(o.jsMap.jsSize())+")")
		keys = inspectObjectKeys(o, c.showHidden)
		braces = [2]string{prefix + "{", "}"}
		if o.jsMap.jsSize() == 0 && len(keys) == 0 {
			return prefix + "{}"
		}
		formatter = func() []string { return c.formatMapEntries(o.jsMap, recurseTimes) }

	case o.jsSet != nil:
		// A Set is the Map case with one value per entry instead of a pair.
		prefix := inspectPrefix(constructor, ctorNull, "Set", "("+strconv.Itoa(o.jsSet.jsSize())+")")
		keys = inspectObjectKeys(o, c.showHidden)
		braces = [2]string{prefix + "{", "}"}
		if o.jsSet.jsSize() == 0 && len(keys) == 0 {
			return prefix + "{}"
		}
		formatter = func() []string { return c.formatSetMembers(o.jsSet, recurseTimes) }

	case o.err != nil:
		keys = inspectErrorKeys(o, c.showHidden)
		base = inspectErrorBase(o.err)
		if len(keys) == 0 {
			return base
		}

	default:
		keys = inspectObjectKeys(o, c.showHidden)
		if ctorNull || constructor != "Object" {
			braces[0] = inspectPrefix(constructor, ctorNull, "Object", "") + "{"
		}
		if len(keys) == 0 {
			return braces[0] + "}"
		}
	}

	if recurseTimes > c.depth {
		name := inspectPrefix(constructor, ctorNull, inspectFallbackName(v), "")
		name = name[:len(name)-1] // drop the trailing space getPrefix leaves
		if !ctorNull {
			name = "[" + name + "]"
		}
		return name
	}
	recurseTimes++

	c.seen = append(c.seen, v.ref)
	c.currentDepth = recurseTimes

	var output []string
	if formatter != nil {
		output = formatter()
	}
	for _, k := range keys {
		output = append(output, c.formatProperty(v, recurseTimes, k, extrasType))
	}

	// The reference marker can only be known now: it is set by a nested
	// [Circular *N] that pointed back at this value while the entries above were
	// being formatted.
	if index, ok := c.circular[v.ref]; ok {
		reference := "<ref *" + strconv.Itoa(index) + ">"
		if base == "" {
			base = reference
		} else {
			base = reference + " " + base
		}
	}
	c.seen = c.seen[:len(c.seen)-1]

	if c.sorted {
		// The entries are sorted after they are rendered, so the order is the order of
		// the printed text rather than of the keys. An array sorts only its named extras
		// and leaves the elements where they are, since an element's position is part of
		// what it means.
		if extrasType == kObjectType {
			c.sortOutput(output)
		} else if len(keys) > 1 {
			c.sortOutput(output[len(output)-len(keys):])
		}
	}

	return c.reduceToSingleString(output, base, braces, extrasType, recurseTimes, v)
}

// inspectFallbackName is the type word a null-prototype value falls back on when
// there is no constructor to name it, Node's internalGetConstructorName.
func inspectFallbackName(v Value) string {
	switch v.kind {
	case KindArray:
		return "Array"
	case KindFunc:
		return "Function"
	default:
		return "Object"
	}
}

// inspectPrefix is Node's getPrefix, the text that precedes the braces and names
// what the value is when its constructor is not the obvious one for its shape. A
// null prototype is called out by name, since an object with no prototype behaves
// differently enough from a plain one that hiding the difference would mislead.
func inspectPrefix(constructor string, ctorNull bool, fallback, size string) string {
	if ctorNull {
		return "[" + fallback + size + ": null prototype] "
	}
	return constructor + size + " "
}

// inspectConstructorName is Node's getConstructorName: it climbs the prototype
// chain looking for a prototype carrying a named constructor function, and reports
// a null prototype as such. bento models Object.prototype as the absence of a
// prototype pointer rather than as a real object, so a chain that runs out without
// an explicit null is the ordinary chain and names the value by its kind.
//
// A regexp and an error are branded on their storage rather than reachable through
// a prototype, so they are named directly; Node reaches the same names through
// RegExp.prototype and the error constructors.
func inspectConstructorName(v Value) (string, bool) {
	if v.asRegExp() != nil {
		return "RegExp", false
	}
	o := v.object()
	if o.err != nil {
		return o.err.ErrorName(), false
	}
	if o.jsMap != nil {
		return "Map", false
	}
	if o.jsSet != nil {
		return "Set", false
	}
	for cur := o; ; {
		if cur.protoNull {
			return "", true
		}
		if cur.proto == nil {
			break
		}
		cur = cur.proto
		if desc, ok := cur.getOwnDesc(FromGoString("constructor")); ok && !desc.accessor && desc.value.kind == KindFunc {
			if name := desc.value.Get(FromGoString("name")); name.kind == KindString && name.str().Length() != 0 {
				return name.str().ToGoString(), false
			}
		}
	}
	return inspectFallbackName(v), false
}

// inspectFunctionBase is Node's getFunctionBase reduced to what bento can tell
// apart. Node reads the function's source text to decide whether to print
// "[class X]"; a lowered bento function has no source to read, so every callable
// reads as a function. The name is the one property that matters here: it is how a
// logged callback is identified at all.
func inspectFunctionBase(v Value, constructor string, ctorNull bool) string {
	base := "[Function"
	if ctorNull {
		base += " (null prototype)"
	}
	if name := v.Get(FromGoString("name")); name.kind == KindString && name.str().Length() != 0 {
		base += ": " + name.str().ToGoString()
	} else {
		base += " (anonymous)"
	}
	base += "]"
	if !ctorNull && constructor != "Function" {
		base += " " + constructor
	}
	return base
}

// inspectErrorBase renders an error the way Node's formatError does for an error
// whose stack holds no frames: the stack text wrapped in brackets. A bento error
// carries no captured stack, so its stack text is exactly the "Name: message" line
// Error.prototype.toString builds, and Node's own output for such an error is
// "[Error: boom]".
func inspectErrorBase(e *Error) string {
	name := e.ErrorName()
	if msg := e.ErrorMessage(); msg != "" {
		return "[" + name + ": " + msg + "]"
	}
	return "[" + name + "]"
}

// inspectErrorKeys are the own properties an error shows after its base text. Node
// drops name and message because both already appear inside the brackets, and
// repeating them would make the common case, an error with a code, three times as
// long as it needs to be.
func inspectErrorKeys(o *Object, showHidden bool) []inspectKey {
	keys := inspectObjectKeys(o, showHidden)
	out := keys[:0]
	for _, k := range keys {
		if k.sym == nil {
			switch k.str.ToGoString() {
			case "name", "message":
				continue
			}
		}
		out = append(out, k)
	}
	return out
}

// inspectObjectKeys is Node's getKeys: the own string keys in the enumeration
// order the language fixes, canonical indices ascending before the rest in
// insertion order, followed by the own symbol keys. showHidden is what decides
// whether a non-enumerable key is one of them, since a key hidden from
// Object.keys is exactly what that option asks to see.
func inspectObjectKeys(o *Object, showHidden bool) []inspectKey {
	names := o.orderedStringKeysFiltered(!showHidden)
	keys := make([]inspectKey, 0, len(names)+len(o.symKeys))
	for _, n := range names {
		keys = append(keys, inspectKey{str: n})
	}
	for i, s := range o.symKeys {
		if showHidden || o.symDescs[i].enumerable {
			keys = append(keys, inspectKey{sym: s})
		}
	}
	return keys
}

// inspectNonIndexKeys is Node's getOwnNonIndexProperties: an array's own
// properties minus its indices, which the element formatter has already rendered.
// Without the filter an array with a named property would print every element
// twice. An array's length is one of those properties, own and non-enumerable, so
// showHidden reports it and prints "[length]: 3" after the elements; bento keeps
// the length implicit in the element slice rather than in the property bag, so it
// is named here rather than found there.
func inspectNonIndexKeys(o *Object, showHidden bool) []inspectKey {
	keys := make([]inspectKey, 0, len(o.keys)+len(o.symKeys)+1)
	if showHidden {
		keys = append(keys, inspectKey{str: FromGoString("length")})
	}
	for i, k := range o.keys {
		if !showHidden && !o.descs[i].enumerable {
			continue
		}
		if _, isIndex := arrayIndex(k.ToGoString()); isIndex {
			continue
		}
		if showHidden && k.ToGoString() == "length" {
			continue // already named above, and naming it twice would print it twice
		}
		keys = append(keys, inspectKey{str: k})
	}
	for i, s := range o.symKeys {
		if showHidden || o.symDescs[i].enumerable {
			keys = append(keys, inspectKey{sym: s})
		}
	}
	return keys
}

// formatProperty is Node's formatProperty. An accessor reports what it is rather
// than being called, since reading a getter to print it would run user code that
// the program did not ask to run at this point. Node's three-space "extra" branch
// cannot fire under the default options (it needs compact set to exactly true), so
// the separator is always the single space.
func (c *inspectCtx) formatProperty(v Value, recurseTimes int, k inspectKey, typ int) string {
	desc, ok := inspectOwnDesc(v.object(), k)
	var str string
	extra := " "
	switch {
	case !ok:
		str = "undefined"
	case desc.accessor && desc.get.kind != KindUndefined:
		label := "Getter"
		if desc.set.kind != KindUndefined {
			label = "Getter/Setter"
		}
		str = c.formatAccessor(v, recurseTimes, desc, label)
	case desc.accessor && desc.set.kind != KindUndefined:
		str = "[Setter]"
	case desc.accessor:
		str = "undefined"
	default:
		// Under compact: true an object property is indented three rather than two, and
		// a value too wide for the line moves below its key instead of beside it. Every
		// other layout indents two and keeps the single space.
		diff := 2
		if c.compactKind == compactTrue && typ == kObjectType {
			diff = 3
		}
		c.indentationLvl += diff
		str = c.formatValue(desc.value, recurseTimes)
		if diff == 3 && c.breakLength < stringWidth(str) {
			extra = "\n" + strings.Repeat(" ", c.indentationLvl)
		}
		c.indentationLvl -= diff
	}
	if typ == kArrayType {
		return str
	}

	var name string
	switch {
	case k.sym != nil:
		name = escapeSequences(symbolValue(k.sym).SymbolDescriptiveString())
	case isPlainKeyName(k.str.ToGoString()):
		if k.str.ToGoString() == "__proto__" {
			name = "['__proto__']"
		} else {
			name = k.str.ToGoString()
		}
	default:
		name = strEscape(k.str)
	}
	// A key hidden from Object.keys is bracketed, so the reader can tell the
	// properties an ordinary walk would find from the ones only showHidden reported.
	if ok && !desc.enumerable {
		name = "[" + name + "]"
	}
	return name + ":" + extra + str
}

// formatAccessor renders one accessor property. With getters off it reports what
// the property is and stops there, which is the default because reading a getter
// runs the program's own code at a point the program did not ask for it: a lazy
// property would be forced, a counter would tick, and printing a value would have
// changed the run. With getters on the accessor is read and its result formatted,
// and a throw from the read is reported in place rather than escaping the inspect,
// since a value that cannot be looked at is still worth naming.
func (c *inspectCtx) formatAccessor(v Value, recurseTimes int, desc descriptor, label string) string {
	read := false
	switch c.getters {
	case gettersAll:
		read = true
	case gettersGet:
		read = desc.set.kind == KindUndefined
	case gettersSet:
		read = desc.set.kind != KindUndefined
	}
	if !read {
		return "[" + label + "]"
	}
	c.indentationLvl += 2
	defer func() { c.indentationLvl -= 2 }()
	tmp, threw := c.callGetter(desc, v)
	switch {
	case threw:
		return "[" + label + ": <Inspection threw (" + c.formatValue(tmp, recurseTimes) + ")>]"
	case tmp.kind == KindNull:
		return "[" + label + ": null]"
	case tmp.kind == KindObject || tmp.kind == KindArray || tmp.kind == KindFunc:
		return "[" + label + "] " + c.formatValue(tmp, recurseTimes)
	default:
		return "[" + label + ": " + c.formatPrimitive(tmp) + "]"
	}
}

// callGetter reads an accessor and reports its result, or what it threw and that it
// threw. A throw arrives as the panic Throw raises, so it is recovered here; a panic
// that is not a JavaScript throw is a bug in the runtime rather than a value the
// program can see, so it keeps unwinding. The flag rather than a nil result is what
// tells the two apart, since a getter is free to throw undefined.
func (c *inspectCtx) callGetter(desc descriptor, receiver Value) (result Value, threw bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		t, ok := r.(Thrown)
		if !ok {
			panic(r)
		}
		result, threw = Caught(t).ToValue(), true
	}()
	return desc.read(receiver), false
}

// inspectOwnDesc reads the descriptor behind a key, whichever bag holds it.
func inspectOwnDesc(o *Object, k inspectKey) (descriptor, bool) {
	if k.sym != nil {
		for i := range o.symKeys {
			if o.symKeys[i] == k.sym {
				return o.symDescs[i], true
			}
		}
		return descriptor{}, false
	}
	if desc, ok := o.getOwnDesc(k.str); ok {
		return desc, true
	}
	// An array's indices live in the element slice rather than the property bag, so
	// a key that names one resolves through the element read.
	if n, isIndex := arrayIndex(k.str.ToGoString()); isIndex && o.kind == KindArray && n < len(o.elems) {
		return dataProperty(o.elems[n], true, true, true), true
	}
	// So does an array's length, which showHidden reports as a property: it is own,
	// writable, and neither enumerable nor configurable, the attributes the spec fixes
	// for it.
	if o.kind == KindArray && k.str.ToGoString() == "length" {
		return dataProperty(Number(float64(len(o.elems))), true, false, false), true
	}
	return descriptor{}, false
}

// isPlainKeyName reports whether a key prints unquoted, Node's keyStrRegExp. It is
// deliberately narrower than the set of valid identifiers: a key starting with a
// digit or holding a dollar sign is quoted, which keeps the rule cheap and errs
// toward showing the key exactly as it is.
func isPlainKeyName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch == '_':
		case ch >= '0' && ch <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// formatArray is Node's formatArray: the elements up to maxArrayLength, with the
// count of the rest named rather than printed. A hole hands the whole array to
// formatSpecialArray, which is the only path that can spell one.
func (c *inspectCtx) formatArray(v Value, recurseTimes int) []string {
	o := v.object()
	valLen := len(o.elems)
	length := valLen
	if c.maxArrayLength < length {
		length = c.maxArrayLength
	}
	if length < 0 {
		length = 0
	}
	remaining := valLen - length

	output := make([]string, 0, length)
	for i := 0; i < length; i++ {
		if isHole(o.elems[i]) {
			return c.formatSpecialArray(v, recurseTimes, length, output, i)
		}
		output = append(output, c.formatElement(o.elems[i], recurseTimes))
	}
	if remaining > 0 {
		output = append(output, remainingText(remaining))
	}
	return output
}

// formatSpecialArray is Node's formatSpecialArray: the sparse case, where the runs
// of missing indices are reported as "<n empty items>" rather than as undefined.
// The difference matters, because a hole and a stored undefined behave differently
// under iteration and printing them the same way would hide that.
func (c *inspectCtx) formatSpecialArray(v Value, recurseTimes, maxLength int, output []string, i int) []string {
	o := v.object()
	keys := o.orderedStringKeysFiltered(true)
	index := i
	for ; i < len(keys) && len(output) < maxLength; i++ {
		key := keys[i].ToGoString()
		n, isIndex := arrayIndex(key)
		if strconv.Itoa(index) != key {
			if !isIndex {
				break // a named property; the caller's key loop renders it
			}
			emptyItems := n - index
			output = append(output, "<"+strconv.Itoa(emptyItems)+" empty item"+plural(emptyItems)+">")
			index = n
			if len(output) == maxLength {
				break
			}
		}
		output = append(output, c.formatElement(o.elems[n], recurseTimes))
		index++
	}
	remaining := len(o.elems) - index
	if len(output) != maxLength {
		if remaining > 0 {
			output = append(output, "<"+strconv.Itoa(remaining)+" empty item"+plural(remaining)+">")
		}
	} else if remaining > 0 {
		output = append(output, remainingText(remaining))
	}
	return output
}

// formatMapEntries is Node's formatMap: one entry per line of output, spelled
// "key => value", both rendered one level deeper in so a nested container that wraps
// lines up inside the braces. The entries are read off the live map in insertion
// order, which is the order a Map enumerates in.
func (c *inspectCtx) formatMapEntries(m mapBacking, recurseTimes int) []string {
	output := make([]string, 0, m.jsSize())
	c.indentationLvl += 2
	for i := 0; i < m.jsSize(); i++ {
		k, v := m.jsEntry(i)
		output = append(output, c.formatValue(k, recurseTimes)+" => "+c.formatValue(v, recurseTimes))
	}
	c.indentationLvl -= 2
	return output
}

// formatSetMembers is Node's formatSet, the Map case with a bare value per entry.
func (c *inspectCtx) formatSetMembers(s setBacking, recurseTimes int) []string {
	output := make([]string, 0, s.jsSize())
	c.indentationLvl += 2
	for i := 0; i < s.jsSize(); i++ {
		output = append(output, c.formatValue(s.jsMember(i), recurseTimes))
	}
	c.indentationLvl -= 2
	return output
}

// formatElement renders one array element, which is formatProperty's array mode:
// the value alone, indented one level further in so a nested container that wraps
// lines up under its opening bracket.
func (c *inspectCtx) formatElement(e Value, recurseTimes int) string {
	c.indentationLvl += 2
	s := c.formatValue(e, recurseTimes)
	c.indentationLvl -= 2
	return s
}

// remainingText and plural are Node's wording for a truncated run.
func remainingText(remaining int) string {
	return "... " + strconv.Itoa(remaining) + " more item" + plural(remaining)
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// formatPrimitive is Node's formatPrimitive. A string is quoted and escaped, a
// number spells its negative zero, a bigint carries its "n", and a symbol renders
// its description rather than throwing the way a string coercion would.
func (c *inspectCtx) formatPrimitive(v Value) string {
	switch v.kind {
	case KindString:
		return c.formatString(v.str())
	case KindNumber:
		return c.formatNumberNoColor(math.Float64frombits(v.scalar))
	case KindBigInt:
		return c.formatBigIntNoColor(v)
	case KindBool:
		if v.scalar != 0 {
			return "true"
		}
		return "false"
	case KindSymbol:
		return v.SymbolDescriptiveString().ToGoString()
	default:
		// undefined, and the hole a sparse array carries where no other path spelled it.
		return "undefined"
	}
}

// inspectNumber is Node's formatNumber. Negative zero is the whole reason this is
// not a plain string coercion: String(-0) is "0", which would hide a sign that
// changes the result of a division and is exactly what someone logging the value
// is trying to see.
func inspectNumber(f float64) string {
	if f == 0 && math.Signbit(f) {
		return "-0"
	}
	return NumberToString(f).ToGoString()
}

// inspectNumberSeparated is the other half of Node's formatNumber, the one the
// numericSeparator option asks for: the digits grouped in threes by underscores, so
// a long count reads as 1_000_000 rather than as a run to be counted by eye. The
// grouping runs from the decimal point outward in both directions, which is why the
// two halves are separated by different helpers, and it leaves a number spelled with
// an exponent alone. Negative zero prints as "0" here, not "-0", because Node
// reaches this path through a plain string coercion, which drops the sign; that is
// the behavior of the option rather than a gap in the port.
func inspectNumberSeparated(f float64) string {
	s := NumberToString(f).ToGoString()
	if math.Trunc(f) == f {
		if math.IsInf(f, 0) || strings.Contains(s, "e") {
			return s
		}
		return addNumericSeparator(s)
	}
	if f != f {
		return s
	}
	dot := strings.IndexByte(s, '.')
	return addNumericSeparator(s[:dot]) + "." + addNumericSeparatorEnd(s[dot+1:])
}

// addNumericSeparator groups an integer's digits in threes from the right, the side
// of the decimal point where the thousands are. A leading minus sign is not a digit
// and is skipped, so -1000 reads as -1_000.
func addNumericSeparator(integerString string) string {
	result := ""
	i := len(integerString)
	start := 0
	if len(integerString) > 0 && integerString[0] == '-' {
		start = 1
	}
	for ; i >= start+4; i -= 3 {
		result = "_" + integerString[i-3:i] + result
	}
	if i == len(integerString) {
		return integerString
	}
	return integerString[:i] + result
}

// addNumericSeparatorEnd groups a fraction's digits in threes from the left, since
// the digit next to the point is the significant one on that side.
func addNumericSeparatorEnd(integerString string) string {
	result := ""
	i := 0
	for ; i < len(integerString)-3; i += 3 {
		result += integerString[i:i+3] + "_"
	}
	if i == 0 {
		return integerString
	}
	return result + integerString[i:]
}

// formatString is Node's string branch of formatPrimitive: a string longer than
// maxStringLength is cut with the remainder counted, and a long multi-line string
// is broken after each newline into quoted pieces joined with " +", so a logged
// template stays readable instead of running off the edge.
func (c *inspectCtx) formatString(s BStr) string {
	units := s.units()
	trailer := ""
	if len(units) > c.maxStringLength {
		remaining := len(units) - c.maxStringLength
		units = units[:c.maxStringLength]
		trailer = "... " + strconv.Itoa(remaining) + " more character" + plural(remaining)
	}
	if c.compactKind != compactTrue &&
		len(units) > kMinLineLength && len(units) > c.breakLength-c.indentationLvl-4 {
		lines := splitAfterNewlines(units)
		parts := make([]string, len(lines))
		for i, line := range lines {
			parts[i] = escapeUnits(line)
		}
		return strings.Join(parts, " +\n"+strings.Repeat(" ", c.indentationLvl+2)) + trailer
	}
	return escapeUnits(units) + trailer
}

// splitAfterNewlines is Node's /(?<=\n)/ split: it cuts after each newline, so
// every piece but possibly the last ends in one and the text reassembles exactly.
func splitAfterNewlines(units []uint16) [][]uint16 {
	var out [][]uint16
	start := 0
	for i, u := range units {
		if u == '\n' {
			out = append(out, units[start:i+1])
			start = i + 1
		}
	}
	if start < len(units) || len(out) == 0 {
		out = append(out, units[start:])
	}
	return out
}

// inspectMeta is Node's meta table: the replacement text for every code unit that
// cannot appear literally inside a quoted string. The blanks are the units that
// need no escape; indexing this table rather than branching is what makes the
// escape loop a single test per unit.
var inspectMeta = [160]string{
	`\x00`, `\x01`, `\x02`, `\x03`, `\x04`, `\x05`, `\x06`, `\x07`,
	`\b`, `\t`, `\n`, `\x0B`, `\f`, `\r`, `\x0E`, `\x0F`,
	`\x10`, `\x11`, `\x12`, `\x13`, `\x14`, `\x15`, `\x16`, `\x17`,
	`\x18`, `\x19`, `\x1A`, `\x1B`, `\x1C`, `\x1D`, `\x1E`, `\x1F`,
	"", "", "", "", "", "", "", `\'`, "", "", "", "", "", "", "", "",
	"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
	"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
	"", "", "", "", "", "", "", "", "", "", "", "", `\\`, "", "", "",
	"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
	"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", `\x7F`,
	`\x80`, `\x81`, `\x82`, `\x83`, `\x84`, `\x85`, `\x86`, `\x87`,
	`\x88`, `\x89`, `\x8A`, `\x8B`, `\x8C`, `\x8D`, `\x8E`, `\x8F`,
	`\x90`, `\x91`, `\x92`, `\x93`, `\x94`, `\x95`, `\x96`, `\x97`,
	`\x98`, `\x99`, `\x9A`, `\x9B`, `\x9C`, `\x9D`, `\x9E`, `\x9F`,
}

// strEscape quotes and escapes a string the way Node does, choosing the quote
// character so the text inside it needs as little escaping as possible: single
// quotes normally, double quotes when the text holds a single quote, backticks
// when it holds both and no template opener. Picking the quote rather than always
// escaping is why "he'llo" prints as "he'llo" with double quotes and not as
// 'he\'llo'.
func strEscape(s BStr) string { return escapeUnits(s.units()) }

func escapeUnits(units []uint16) string {
	singleQuote := 39
	if containsUnit(units, '\'') {
		switch {
		case !containsUnit(units, '"'):
			singleQuote = -1
		case !containsUnit(units, '`') && !containsTemplateOpen(units):
			singleQuote = -2
		}
	}

	var out []uint16
	last := 0
	for i := 0; i < len(units); i++ {
		point := int(units[i])
		switch {
		case point == singleQuote || point == 92 || point < 32 || (point > 126 && point < 160):
			out = append(out, units[last:i]...)
			out = appendASCII(out, inspectMeta[point])
			last = i + 1
		case point >= 0xd800 && point <= 0xdfff:
			if point <= 0xdbff && i+1 < len(units) {
				if next := int(units[i+1]); next >= 0xdc00 && next <= 0xdfff {
					i++ // a well-formed pair stands as it is
					continue
				}
			}
			// A lone surrogate cannot be written literally, so it is spelled out.
			out = append(out, units[last:i]...)
			out = appendASCII(out, `\u`+strconv.FormatInt(int64(point), 16))
			last = i + 1
		}
	}
	if last != len(units) {
		out = append(out, units[last:]...)
	}
	return addQuotes(string(utf16.Decode(out)), singleQuote)
}

// escapeSequences applies the same escapes with no quoting, which is what a symbol
// key gets: Symbol(a\nb) must not put a real newline in the middle of a property
// list, but it is not a quoted string either.
func escapeSequences(s BStr) string {
	units := s.units()
	var out []uint16
	last := 0
	for i := 0; i < len(units); i++ {
		point := int(units[i])
		if point == 39 || point == 92 || point < 32 || (point > 126 && point < 160) {
			out = append(out, units[last:i]...)
			out = appendASCII(out, inspectMeta[point])
			last = i + 1
		}
	}
	if last != len(units) {
		out = append(out, units[last:]...)
	}
	return string(utf16.Decode(out))
}

func addQuotes(s string, singleQuote int) string {
	switch singleQuote {
	case -1:
		return `"` + s + `"`
	case -2:
		return "`" + s + "`"
	default:
		return "'" + s + "'"
	}
}

func containsUnit(units []uint16, u uint16) bool {
	for _, x := range units {
		if x == u {
			return true
		}
	}
	return false
}

func containsTemplateOpen(units []uint16) bool {
	for i := 0; i+1 < len(units); i++ {
		if units[i] == '$' && units[i+1] == '{' {
			return true
		}
	}
	return false
}

func appendASCII(dst []uint16, s string) []uint16 {
	for i := 0; i < len(s); i++ {
		dst = append(dst, uint16(s[i]))
	}
	return dst
}

// isBelowBreakLength is Node's isBelowBreakLength: whether the entries plus their
// separators fit the terminal width. The count is added twice at the start, once
// for the comma each entry needs and once for the space after it.
func (c *inspectCtx) isBelowBreakLength(output []string, start int, base string) bool {
	totalLength := len(output) + start
	if totalLength+len(output) > c.breakLength {
		return false
	}
	for _, s := range output {
		totalLength += u16Len(s)
		if totalLength > c.breakLength {
			return false
		}
	}
	return base == "" || !strings.Contains(base, "\n")
}

// reduceToSingleString is Node's reduceToSingleString: it decides between one line
// and one entry per line. The depth test is what keeps a shallow object compact
// while an object holding deep structure spreads out, and it reads currentDepth,
// the deepest level this subtree reached, rather than the current level.
func (c *inspectCtx) reduceToSingleString(output []string, base string, braces [2]string, extrasType, recurseTimes int, v Value) string {
	if c.compactKind == compactTrue {
		return c.reduceCompact(output, base, braces)
	}
	if c.compactKind == compactNumber && c.compact >= 1 {
		entries := len(output)
		if extrasType == kArrayExtrasType && entries > 6 {
			output = c.groupArrayElements(output, v)
		}
		if c.currentDepth-recurseTimes < c.compact && entries == len(output) {
			// The added ten is Node's constant, standing in for the other things that eat
			// into the width before the entries are reached.
			start := len(output) + c.indentationLvl + u16Len(braces[0]) + u16Len(base) + 10
			if c.isBelowBreakLength(output, start, base) {
				joined := strings.Join(output, ", ")
				if !strings.Contains(joined, "\n") {
					return basePrefix(base) + braces[0] + " " + joined + " " + braces[1]
				}
			}
		}
	}
	indentation := "\n" + strings.Repeat(" ", c.indentationLvl)
	return basePrefix(base) + braces[0] + indentation + "  " +
		strings.Join(output, ","+indentation+"  ") + indentation + braces[1]
}

// reduceCompact is the compact: true layout, the oldest of the three. It puts every
// entry on one line whenever they fit, however deep the structure is, and when they
// do not it indents them under a first line that carries the brace, which is what
// keeps the entries lined up when the opening brace is a word rather than a bracket.
func (c *inspectCtx) reduceCompact(output []string, base string, braces [2]string) string {
	if c.isBelowBreakLength(output, 0, base) {
		return braces[0] + basePrefixInner(base) + " " + strings.Join(output, ", ") + " " + braces[1]
	}
	indentation := strings.Repeat(" ", c.indentationLvl)
	ln := " "
	if base != "" || u16Len(braces[0]) != 1 {
		ln = basePrefixInner(base) + "\n" + indentation + "  "
	}
	return braces[0] + ln + strings.Join(output, ",\n"+indentation+"  ") + " " + braces[1]
}

// basePrefixInner is the base as it reads after the opening brace rather than
// before it, the spacing the compact: true layout uses.
func basePrefixInner(base string) string {
	if base == "" {
		return ""
	}
	return " " + base
}

func basePrefix(base string) string {
	if base == "" {
		return ""
	}
	return base + " "
}

// groupArrayElements is Node's groupArrayElements: a long array of short entries
// is laid out in columns rather than one entry per line, so a hundred numbers read
// as a block instead of a hundred-line wall. The arithmetic is Node's, including
// the assumption that a character is about two and a half times as tall as it is
// wide, which is what makes the block come out roughly square.
func (c *inspectCtx) groupArrayElements(output []string, v Value) []string {
	totalLength := 0
	maxLength := 0
	outputLength := len(output)
	if c.maxArrayLength < len(output) {
		// The trailing "... n more items" is not one of the columns.
		outputLength--
	}
	const separatorSpace = 2 // one for the comma and one for the space
	dataLen := make([]int, outputLength)
	for i := 0; i < outputLength; i++ {
		length := stringWidth(output[i])
		dataLen[i] = length
		totalLength += length + separatorSpace
		if maxLength < length {
			maxLength = length
		}
	}
	actualMax := maxLength + separatorSpace

	// Group only when at least three entries fit side by side, and not when one
	// entry dwarfs the rest, since the padding would then be wider than the data.
	if actualMax*3+c.indentationLvl >= c.breakLength ||
		(float64(totalLength)/float64(actualMax) <= 5 && maxLength > 6) {
		return output
	}

	const approxCharHeights = 2.5
	averageBias := math.Sqrt(float64(actualMax) - float64(totalLength)/float64(len(output)))
	biasedMax := math.Max(float64(actualMax)-3-averageBias, 1)
	columns := int(math.Min(math.Min(
		// The area of a square holding outputLength cells of the entry size, divided
		// by the entry width, is the column count that comes closest to a square.
		math.Round(math.Sqrt(approxCharHeights*biasedMax*float64(outputLength))/biasedMax),
		math.Floor(float64(c.breakLength-c.indentationLvl)/float64(actualMax))),
		math.Min(float64(c.compact*4), 15)))
	if columns <= 1 {
		return output
	}

	maxLineLength := make([]int, columns)
	for i := 0; i < columns; i++ {
		lineMaxLength := 0
		for j := i; j < len(output); j += columns {
			if j < len(dataLen) && dataLen[j] > lineMaxLength {
				lineMaxLength = dataLen[j]
			}
		}
		maxLineLength[i] = lineMaxLength + separatorSpace
	}

	// Numbers line up on their right edge so their digits align; anything else lines
	// up on the left, since a right-aligned word reads as ragged.
	padStart := true
	if v.kind == KindArray {
		o := v.object()
		for i := 0; i < len(output) && i < len(o.elems); i++ {
			if k := o.elems[i].kind; k != KindNumber && k != KindBigInt {
				padStart = false
				break
			}
		}
	} else {
		padStart = false
	}

	var tmp []string
	for i := 0; i < outputLength; i += columns {
		max := i + columns
		if max > outputLength {
			max = outputLength
		}
		str := ""
		j := i
		for ; j < max-1; j++ {
			padding := maxLineLength[j-i] + u16Len(output[j]) - dataLen[j]
			str += padTo(output[j]+", ", padding, padStart)
		}
		if padStart {
			padding := maxLineLength[j-i] + u16Len(output[j]) - dataLen[j] - separatorSpace
			str += padTo(output[j], padding, true)
		} else {
			str += output[j]
		}
		tmp = append(tmp, str)
	}
	if c.maxArrayLength < len(output) {
		tmp = append(tmp, output[outputLength])
	}
	return tmp
}

// padTo is String.prototype.padStart and padEnd, which count in code units.
func padTo(s string, target int, atStart bool) string {
	fill := target - u16Len(s)
	if fill <= 0 {
		return s
	}
	if atStart {
		return strings.Repeat(" ", fill) + s
	}
	return s + strings.Repeat(" ", fill)
}

// u16Len is the length JavaScript reports for a string, in UTF-16 code units,
// which is what every width calculation in Node's layout is measured in.
func u16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// stringWidth is Node's getStringWidth: the columns a string occupies on a
// terminal, which is not its length. An East Asian character takes two columns and
// a combining mark takes none, so a grouped array holding them would come out
// ragged if the padding counted units. This is the fallback Node uses when it was
// built without ICU; the ICU path differs only on characters whose width is
// ambiguous, and it does not normalize, which Node's fallback does.
func stringWidth(s string) int {
	width := 0
	for _, r := range s {
		switch {
		case isFullWidthCodePoint(r):
			width += 2
		case isZeroWidthCodePoint(r):
		default:
			width++
		}
	}
	return width
}

func isZeroWidthCodePoint(r rune) bool {
	return r <= 0x1F ||
		(r >= 0x7F && r <= 0x9F) ||
		(r >= 0x300 && r <= 0x36F) ||
		(r >= 0x200B && r <= 0x200F) ||
		(r >= 0x20D0 && r <= 0x20FF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0xFE20 && r <= 0xFE2F) ||
		(r >= 0xE0100 && r <= 0xE01EF)
}

func isFullWidthCodePoint(r rune) bool {
	if r < 0x1100 {
		return false
	}
	return r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0x3247 && r != 0x303f) ||
		(r >= 0x3250 && r <= 0x4dbf) ||
		(r >= 0x4e00 && r <= 0xa4c6) ||
		(r >= 0xa960 && r <= 0xa97c) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6b) ||
		(r >= 0xff01 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1b000 && r <= 0x1b001) ||
		(r >= 0x1f200 && r <= 0x1f251) ||
		(r >= 0x1f300 && r <= 0x1f64f) ||
		(r >= 0x20000 && r <= 0x3fffd)
}
