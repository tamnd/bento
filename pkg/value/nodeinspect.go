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
// arrays, functions, regexps, errors, and proxies. Map, Set, Date and the typed
// arrays are concrete Go types that do not box into a Value yet, so their
// branches in Node's formatRaw have no counterpart here and are named in the
// implementation note rather than half-written.

package value

import (
	"math"
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

// inspectCtx is Node's ctx object: the options plus the mutable bookkeeping the
// recursion threads through itself. seen is the stack of references currently
// being formatted, which is how a cycle is caught; circular numbers the references
// a cycle pointed back at, so the "<ref *1>" marker and the "[Circular *1]" that
// refers to it agree. currentDepth is the depth the deepest nested formatRaw
// reached, which is what decides whether a level may collapse onto one line, and
// it is deliberately not restored on the way back up, matching Node.
type inspectCtx struct {
	indentationLvl  int
	seen            []unsafe.Pointer
	currentDepth    int
	circular        map[unsafe.Pointer]int
	depth           int
	breakLength     int
	compact         int
	maxArrayLength  int
	maxStringLength int
}

// NodeInspect renders a value the way Node's util.inspect does with its default
// options, which is what console.log prints for every argument that is not already
// a string. It is the console's renderer rather than a string coercion: an object
// reads as its properties instead of "[object Object]", a string nested in a
// container is quoted, and a bigint carries its "n".
func NodeInspect(v Value) BStr {
	c := &inspectCtx{
		depth:           inspectDepth,
		breakLength:     inspectBreakLength,
		compact:         inspectCompact,
		maxArrayLength:  inspectMaxArrayLength,
		maxStringLength: inspectMaxStringLength,
	}
	return FromGoString(c.formatValue(v, 0))
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
		keys = inspectNonIndexKeys(o)
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
		keys = inspectObjectKeys(o)
		base = inspectFunctionBase(v, constructor, ctorNull)
		if len(keys) == 0 {
			return base
		}

	case v.asRegExp() != nil:
		keys = inspectObjectKeys(o)
		base = v.asRegExp().ToStringBStr().ToGoString()
		if len(keys) == 0 || recurseTimes > c.depth {
			return base
		}

	case o.err != nil:
		keys = inspectErrorKeys(o)
		base = inspectErrorBase(o.err)
		if len(keys) == 0 {
			return base
		}

	default:
		keys = inspectObjectKeys(o)
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
func inspectErrorKeys(o *Object) []inspectKey {
	keys := inspectObjectKeys(o)
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

// inspectObjectKeys is Node's getKeys with showHidden off: the own enumerable
// string keys in the enumeration order the language fixes, canonical indices
// ascending before the rest in insertion order, followed by the own enumerable
// symbol keys.
func inspectObjectKeys(o *Object) []inspectKey {
	names := o.orderedStringKeysFiltered(true)
	keys := make([]inspectKey, 0, len(names)+len(o.symKeys))
	for _, n := range names {
		keys = append(keys, inspectKey{str: n})
	}
	for i, s := range o.symKeys {
		if o.symDescs[i].enumerable {
			keys = append(keys, inspectKey{sym: s})
		}
	}
	return keys
}

// inspectNonIndexKeys is Node's getOwnNonIndexProperties: an array's own
// enumerable properties minus its indices, which the element formatter has already
// rendered. Without the filter an array with a named property would print every
// element twice.
func inspectNonIndexKeys(o *Object) []inspectKey {
	keys := make([]inspectKey, 0, len(o.keys)+len(o.symKeys))
	for i, k := range o.keys {
		if !o.descs[i].enumerable {
			continue
		}
		if _, isIndex := arrayIndex(k.ToGoString()); isIndex {
			continue
		}
		keys = append(keys, inspectKey{str: k})
	}
	for i, s := range o.symKeys {
		if o.symDescs[i].enumerable {
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
	switch {
	case !ok:
		str = "undefined"
	case desc.accessor && desc.get.kind != KindUndefined && desc.set.kind != KindUndefined:
		str = "[Getter/Setter]"
	case desc.accessor && desc.get.kind != KindUndefined:
		str = "[Getter]"
	case desc.accessor && desc.set.kind != KindUndefined:
		str = "[Setter]"
	case desc.accessor:
		str = "undefined"
	default:
		c.indentationLvl += 2
		str = c.formatValue(desc.value, recurseTimes)
		c.indentationLvl -= 2
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
	return name + ": " + str
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
		return inspectNumber(math.Float64frombits(v.scalar))
	case KindBigInt:
		return v.bigint().String() + "n"
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
	if len(units) > kMinLineLength && len(units) > c.breakLength-c.indentationLvl-4 {
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
	indentation := "\n" + strings.Repeat(" ", c.indentationLvl)
	return basePrefix(base) + braces[0] + indentation + "  " +
		strings.Join(output, ","+indentation+"  ") + indentation + braces[1]
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
