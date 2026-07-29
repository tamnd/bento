package value

import "strings"

// This file is util.format, ported statement for statement from Node's
// lib/internal/util/inspect.js (formatWithOptionsInternal and the helpers it
// reaches for). It is what console.log('%s: %d', name, n) prints and what
// require('util').format returns, so it has to agree with Node down to the
// spacing.
//
// The rules are not guessable, which is why this is a port and why the tests are
// real Node output rather than a reading of the docs. A sample of what had to come
// out right:
//
//	format('%s %s', 'a')      -> "a %s"     a specifier past the last argument stays literal
//	format('%%', 'x')         -> "% x"      an escape still consumes no argument, and the
//	                                        leftover argument is appended with a space
//	format('a%%b')            -> "a%%b"     one argument returns the string untouched, so
//	                                        the escape is never collapsed
//	format('%d', -0)          -> "-0"       the console keeps a sign String() drops
//	format('%i', '42.9px')    -> "42"       parseInt, not Number
//	format('%f', 10n)         -> "10"       parseFloat of the bigint's digits, no "n"
//	format('%d', 10n)         -> "10n"      but %d keeps it
//	format('%d', Symbol())    -> "NaN"      rather than the TypeError a coercion throws
//	format('%o', [1, 2, 3])   -> "[ 1, 2, 3, [length]: 3 ]"
//
// Every specifier consumes one argument except %% (which consumes none) and %c
// (which consumes one and prints nothing, since bento has no CSS to apply). What is
// left over after the format string is appended space-separated, inspected unless it
// is already a string.

// NodeFormat is util.format: the format string's specifiers filled from the
// arguments that follow, with the leftovers appended. It is also what console.log
// does with its arguments, because console's no-color options are the defaults.
func NodeFormat(args ...Value) BStr {
	return FromGoString(formatWithOptionsInternal(defaultInspectOptions(), args))
}

// NodeFormatWithOptions is util.formatWithOptions: the same formatting with the
// inspect options the caller chose, which reach the %s, %o and %O specifiers and
// every leftover argument. The options argument is validated rather than ignored,
// because Node throws ERR_INVALID_ARG_TYPE on a non-object and a program that
// passes its format string there by mistake should learn so from the same error.
func NodeFormatWithOptions(opts Value, args ...Value) BStr {
	requireObject(opts, "inspectOptions")
	o := defaultInspectOptions()
	o.readOptions(opts)
	return FromGoString(formatWithOptionsInternal(o, args))
}

// ConsoleFormat renders one console.log or console.error call's arguments, the
// single string that goes to the stream. console.log is util.format with color
// options when it writes to a terminal and without them otherwise, and bento does
// not color, so this is util.format.
func ConsoleFormat(args ...Value) BStr {
	return NodeFormat(args...)
}

// formatWithOptionsInternal is Node's formatWithOptionsInternal. It walks the
// format string's code units looking for a percent sign, and the index arithmetic
// is worth reading closely because it is what makes the odd cases above come out
// right: lastPos marks how much of the string has been copied out, and it stays 0
// until a specifier is actually substituted, which is how a string with no live
// specifier is emitted whole (escapes and all) and how the join before the leftover
// arguments is decided.
func formatWithOptionsInternal(o inspectOptions, args []Value) string {
	a := 0
	str := ""
	join := ""

	if len(args) > 0 && args[0].kind == KindString {
		if len(args) == 1 {
			return args[0].str().ToGoString()
		}
		first := args[0].str().units()
		tempStr := ""
		lastPos := 0

		for i := 0; i < len(first)-1; i++ {
			if first[i] == '%' {
				i++
				nextChar := first[i]
				if a+1 != len(args) {
					switch nextChar {
					case 's':
						a++
						tempStr = o.formatSpecifierS(args[a])
					case 'j':
						a++
						tempStr = tryStringify(args[a])
					case 'd':
						a++
						switch tempNum := args[a]; tempNum.kind {
						case KindBigInt:
							tempStr = o.formatBigIntNoColor(tempNum)
						case KindSymbol:
							tempStr = "NaN"
						default:
							tempStr = o.formatNumberNoColor(ToNumber(tempNum))
						}
					case 'O':
						a++
						tempStr = inspectWith(o, args[a])
					case 'o':
						a++
						oo := o
						oo.showHidden = true
						oo.showProxy = true
						oo.depth = 4
						tempStr = inspectWith(oo, args[a])
					case 'i':
						a++
						switch tempInteger := args[a]; tempInteger.kind {
						case KindBigInt:
							tempStr = o.formatBigIntNoColor(tempInteger)
						case KindSymbol:
							tempStr = "NaN"
						default:
							tempStr = o.formatNumberNoColor(ParseInt(ToString(tempInteger), 0))
						}
					case 'f':
						a++
						if tempFloat := args[a]; tempFloat.kind == KindSymbol {
							tempStr = "NaN"
						} else {
							tempStr = o.formatNumberNoColor(ParseFloat(ToString(tempFloat)))
						}
					case 'c':
						// %c is a CSS rule in a browser console. Node consumes the argument
						// and prints nothing, which is what a program written for a browser
						// should get: the styling silently dropped rather than dumped into
						// the output as text.
						a++
						tempStr = ""
					case '%':
						str += FromUTF16(first[lastPos:i]).ToGoString()
						lastPos = i + 1
						continue
					default:
						// Not a specifier at all, so the percent sign and the character after
						// it are ordinary text and are copied out by a later slice.
						continue
					}
					if lastPos != i-1 {
						str += FromUTF16(first[lastPos : i-1]).ToGoString()
					}
					str += tempStr
					lastPos = i + 1
				} else if nextChar == '%' {
					// The arguments are used up, so no specifier substitutes here, but an
					// escape still collapses: format('%s %%', 'a') ends in a single percent.
					str += FromUTF16(first[lastPos:i]).ToGoString()
					lastPos = i + 1
				}
			}
		}
		if lastPos != 0 {
			a++
			join = " "
			if lastPos < len(first) {
				str += FromUTF16(first[lastPos:]).ToGoString()
			}
		}
	}

	for a < len(args) {
		v := args[a]
		str += join
		if v.kind == KindString {
			str += v.str().ToGoString()
		} else {
			str += inspectWith(o, v)
		}
		join = " "
		a++
	}
	return str
}

// formatSpecifierS is the %s branch. A number and a bigint take the console's
// number spelling rather than a string coercion, so -0 keeps its sign and the
// numericSeparator option is honored. Everything that is not an object takes
// String(), which is why %s on a function prints its source and %s on a symbol
// prints its description instead of throwing. An object takes String() too when it
// brought its own toString, since that is the text its author wrote it to have, and
// is otherwise inspected one level deep.
func (o inspectOptions) formatSpecifierS(v Value) string {
	switch {
	case v.kind == KindNumber:
		return o.formatNumberNoColor(v.AsNumber())
	case v.kind == KindBigInt:
		return o.formatBigIntNoColor(v)
	case v.kind == KindFunc:
		// Node prints the function's source text here, because that is what
		// Function.prototype.toString returns. An AOT compiler has no source text at
		// run time: the function is Go code by then, and keeping every function
		// literal's text in the binary to satisfy a log line is not a trade worth
		// making. The inspect form is printed instead, so %s on a function reads
		// "[Function: foo]" rather than the "[object Object]" a string coercion of a
		// function produces today. It names the function, which is what the caller
		// was almost certainly after.
		return inspectWith(o, v)
	case !isFormatObject(v) || !hasBuiltInToString(v):
		return StringCoerce(v).ToGoString()
	default:
		so := o
		so.compact = 3
		so.compactKind = compactNumber
		so.depth = 0
		return inspectWith(so, v)
	}
}

// isFormatObject is Node's `typeof value === 'object' && value !== null` test on
// the %s argument. A function is not an object here, matching typeof, though the
// case above has already answered for one.
func isFormatObject(v Value) bool {
	return v.kind == KindObject || v.kind == KindArray
}

// hasBuiltInToString reports whether the value's toString is the one it inherited
// from a built-in prototype rather than one its author wrote. %s asks because the
// answer decides between two very different renderings: an object with its own
// toString is printed as that method's text, and an object without one is inspected,
// which is how console.log('%s', { a: 1 }) prints "{ a: 1 }" instead of
// "[object Object]".
//
// Node walks the prototype chain to the object that owns toString or
// Symbol.toPrimitive and asks whether that object's constructor is a built-in. In
// bento the walk is shorter for a structural reason: the runtime has no prototype
// object for Object, Array, Error or RegExp, so any toString reachable from a value
// was written by the program. Finding one anywhere on the chain therefore answers
// the same question as Node's constructor test, and reaching the end of the chain is
// reaching Object.prototype, whose toString is built in.
func hasBuiltInToString(v Value) bool {
	o := v.object()
	// A proxy without a get trap forwards to its target, and one with a trap could
	// synthesize a toString, but running a trap to decide how to print is a side
	// effect the caller did not ask for. Reading through to the target keeps the
	// common case (a proxy over a plain object) printing as the object.
	for o.proxy != nil {
		if !isObjectLike(o.proxy.target) {
			return true
		}
		o = o.proxy.target.object()
	}
	for p := o; p != nil; p = p.proto {
		if p.hasOwn(toStringKey) || p.hasSym(symbolToPrimitive) {
			return false
		}
	}
	return true
}

// toStringKey is the "toString" property name, hoisted so the %s path does not
// rebuild it per argument.
var toStringKey = FromGoString("toString")

// tryStringify is the %j specifier: JSON.stringify of the argument, with a cycle
// reported as [Circular] rather than propagating the throw, since a log line that
// crashes the program is worse than one that says the value repeats. A value whose
// JSON form is undefined (undefined itself, a function, a symbol) prints
// "undefined", which is what Node's string concatenation of the undefined result
// produces.
//
// Only the circular TypeError is swallowed. Any other throw, for instance from a
// toJSON hook the program wrote, propagates the way it does in Node.
func tryStringify(v Value) (out string) {
	if jsonUndefinedValue(v) {
		return "undefined"
	}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		t, ok := r.(Thrown)
		if !ok {
			panic(r)
		}
		e := Caught(t)
		if e.ErrorName() != "TypeError" || firstErrorLine(e) != circularErrorMessage {
			panic(r)
		}
		out = "[Circular]"
	}()
	var b strings.Builder
	encodeBoxedJSON(&b, v, nil)
	return b.String()
}

// circularErrorMessage is the first line of the TypeError JSON.stringify throws on
// a cycle. Node computes it once by stringifying a self-referencing object rather
// than writing it down, so a change to the engine's wording cannot make %j start
// re-throwing; bento owns both sides, so the literal is the same contract with one
// fewer moving part, and a test pins the two together.
const circularErrorMessage = "Converting circular structure to JSON"

// firstErrorLine takes the first line of an error's message, which is all the
// circular test compares. V8's circular error carries several more lines naming the
// property path that closes the loop, and those depend on the value, so only the
// first line is stable enough to match on.
func firstErrorLine(e *Error) string {
	msg := e.ErrorMessage()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}
	return msg
}

// formatNumberNoColor is Node's formatNumber without the color wrapper: the
// console's number spelling, which keeps negative zero's sign and groups digits
// when the numericSeparator option is on.
func (o inspectOptions) formatNumberNoColor(f float64) string {
	if o.numericSeparator {
		return inspectNumberSeparated(f)
	}
	return inspectNumber(f)
}

// formatBigIntNoColor is the bigint half of the same, the digits with the "n"
// suffix that tells a bigint from a number.
func (o inspectOptions) formatBigIntNoColor(v Value) string {
	s := v.bigint().String()
	if o.numericSeparator {
		return addNumericSeparator(s) + "n"
	}
	return s + "n"
}
