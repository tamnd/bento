// This file gives a string its prototype methods on the dynamic path, the String half
// of what primitivemember.go does for a number, a boolean, and a bigint.
//
// The statically typed path never reaches here. A receiver the checker types as a
// string has s.slice(1) lowered straight to the value.BStr method by calls.go, with the
// argument kinds checked at compile time. What arrives here is the other case, a
// receiver the checker typed any, which is most of what a Node program holds: a
// parameter of a boxed object method, an element read off a dynamic array, and nearly
// everything a Node API hands back, since bento's ambient declarations type it any.
//
// Without this a dynamic s.toUpperCase() read undefined off the string and the call
// threw "undefined is not a function", so a program that compiled could not run. Every
// member here delegates to the same value.BStr or *RegExp method the static path emits,
// so a string operated on through a dynamic call and one operated on through a lowered
// call give the same answer and raise the same errors from the same place.
//
// The surface is the one stringMethod (pkg/lower/calls.go) lowers, plus the four
// regexp-taking methods regexp_string.go implements. A name outside it reports false and
// reads as undefined, the same miss any other receiver gives, rather than a silently
// wrong answer: localeCompare and the toLocale case mappings need locale data bento does
// not carry, and matchAll needs an iterator no path builds yet.

package value

// stringGet answers a member read off a string, the dynamic half of String.prototype.
// It reports false for a name that is not one of the methods, so the read falls through
// to undefined the way a miss on any other receiver does. The length property and the
// index reads are answered before this by Get, since those are data properties rather
// than methods.
func stringGet(s BStr, name string) (Value, bool) {
	switch name {
	case "toString", "valueOf":
		// Both are identity on a primitive string, the same shortcut the borrowed-call
		// path in calls.go takes.
		return boundMethod(name, func(args []Value) Value { return StringValue(s) }), true

	case "at":
		// at answers string | undefined: a negative index counts from the end and an
		// out-of-range one reads as undefined.
		return boundMethod(name, func(args []Value) Value {
			return OptToValue(s.AtOpt(ToNumber(Arg(args, 0))), StringValue)
		}), true
	case "charAt":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.CharAt(ToNumber(Arg(args, 0))))
		}), true
	case "charCodeAt":
		return boundMethod(name, func(args []Value) Value {
			return Number(s.CharCodeAt(ToNumber(Arg(args, 0))))
		}), true
	case "codePointAt":
		return boundMethod(name, func(args []Value) Value {
			return OptToValue(s.CodePointAtOpt(ToNumber(Arg(args, 0))), Number)
		}), true

	case "indexOf":
		return boundMethod(name, func(args []Value) Value {
			return Number(s.IndexOf(ToString(Arg(args, 0)), strNumTail(args, 1, 1)...))
		}), true
	case "lastIndexOf":
		return boundMethod(name, func(args []Value) Value {
			return Number(s.LastIndexOf(ToString(Arg(args, 0)), strNumTail(args, 1, 1)...))
		}), true
	case "includes":
		return boundMethod(name, func(args []Value) Value {
			return Bool(s.Includes(ToString(Arg(args, 0)), strNumTail(args, 1, 1)...))
		}), true
	case "startsWith":
		return boundMethod(name, func(args []Value) Value {
			return Bool(s.StartsWith(ToString(Arg(args, 0)), strNumTail(args, 1, 1)...))
		}), true
	case "endsWith":
		return boundMethod(name, func(args []Value) Value {
			return Bool(s.EndsWith(ToString(Arg(args, 0)), strNumTail(args, 1, 1)...))
		}), true

	case "slice":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.Slice(strNumTail(args, 0, 2)...))
		}), true
	case "substring":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.Substring(strNumTail(args, 0, 2)...))
		}), true
	case "substr":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.Substr(strNumTail(args, 0, 2)...))
		}), true

	case "trim":
		return boundMethod(name, func(args []Value) Value { return StringValue(s.Trim()) }), true
	case "trimStart":
		return boundMethod(name, func(args []Value) Value { return StringValue(s.TrimStart()) }), true
	case "trimEnd":
		return boundMethod(name, func(args []Value) Value { return StringValue(s.TrimEnd()) }), true
	case "toUpperCase":
		return boundMethod(name, func(args []Value) Value { return StringValue(s.ToUpperCase()) }), true
	case "toLowerCase":
		return boundMethod(name, func(args []Value) Value { return StringValue(s.ToLowerCase()) }), true
	case "isWellFormed":
		return boundMethod(name, func(args []Value) Value { return Bool(s.IsWellFormed()) }), true
	case "toWellFormed":
		return boundMethod(name, func(args []Value) Value { return StringValue(s.ToWellFormed()) }), true

	case "normalize":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.Normalize(strStrTail(args, 0, 1)...))
		}), true
	case "repeat":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.Repeat(ToNumber(Arg(args, 0))))
		}), true
	case "padStart":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.PadStart(ToNumber(Arg(args, 0)), strStrTail(args, 1, 1)...))
		}), true
	case "padEnd":
		return boundMethod(name, func(args []Value) Value {
			return StringValue(s.PadEnd(ToNumber(Arg(args, 0)), strStrTail(args, 1, 1)...))
		}), true
	case "concat":
		return boundMethod(name, func(args []Value) Value {
			rest := make([]BStr, len(args))
			for i, a := range args {
				rest[i] = ToString(a)
			}
			return StringValue(s.ConcatN(rest...))
		}), true

	case "search":
		return boundMethod(name, func(args []Value) Value {
			return Number(stringRegExpArg(Arg(args, 0)).Search(s))
		}), true
	case "match":
		return boundMethod(name, func(args []Value) Value {
			return stringRegExpArg(Arg(args, 0)).MatchStr(s)
		}), true
	case "split":
		return boundMethod(name, func(args []Value) Value { return stringSplit(s, args) }), true
	case "replace":
		return boundMethod(name, func(args []Value) Value { return stringReplace(s, args, false) }), true
	case "replaceAll":
		return boundMethod(name, func(args []Value) Value { return stringReplace(s, args, true) }), true
	}
	return Undefined, false
}

// strNumTail coerces the numeric arguments from index start onward, at most max of
// them, for a Go-variadic BStr method. A trailing undefined is dropped rather than
// coerced, because these methods read an omitted argument as their own default while
// ToNumber would hand them NaN: "abc".slice(1, undefined) is "bc", not the empty string
// a NaN end would cut. An undefined that is not trailing does coerce, to the NaN the
// index rules read as zero, which is the ToIntegerOrInfinity the specification applies.
func strNumTail(args []Value, start, max int) []float64 {
	n := len(args)
	if n > start+max {
		n = start + max
	}
	for n > start && args[n-1].kind == KindUndefined {
		n--
	}
	if n <= start {
		return nil
	}
	out := make([]float64, 0, n-start)
	for _, a := range args[start:n] {
		out = append(out, ToNumber(a))
	}
	return out
}

// strStrTail is strNumTail for a string-valued optional argument, the pad of padStart
// and the form of normalize. An omitted or undefined argument is dropped so the method
// applies its own default, a single space and NFC respectively, rather than padding
// with the text "undefined" or normalizing to a form of that name.
func strStrTail(args []Value, start, max int) []BStr {
	n := len(args)
	if n > start+max {
		n = start + max
	}
	for n > start && args[n-1].kind == KindUndefined {
		n--
	}
	if n <= start {
		return nil
	}
	out := make([]BStr, 0, n-start)
	for _, a := range args[start:n] {
		out = append(out, ToString(a))
	}
	return out
}

// stringRegExpArg resolves the argument of match, search, or a regexp-shaped split or
// replace to a regexp. A regexp box is used as it is, so its flags and its lastIndex
// are the live ones; anything else is compiled as a pattern, which is what the
// specification's ToRegExp step does when it hands a non-regexp to the RegExp
// constructor. An omitted argument compiles the empty pattern, which matches at
// position zero the way new RegExp(undefined) does.
func stringRegExpArg(v Value) *RegExp {
	if re := v.asRegExp(); re != nil {
		return re
	}
	if v.kind == KindUndefined {
		return NewRegExpLiteral("", "")
	}
	return NewRegExpLiteral(ToString(v).ToGoString(), "")
}

// stringSplit runs String.prototype.split off a dynamic receiver. A regexp separator
// delegates to the regexp splitter, which already answers a boxed array; a string
// separator runs the BStr splitter and boxes its string array. An omitted separator
// does not split at all and yields the whole string as the single element, unless a
// zero limit caps the result away.
func stringSplit(s BStr, args []Value) Value {
	sep, lim := Arg(args, 0), Arg(args, 1)
	limited := lim.kind != KindUndefined
	if re := sep.asRegExp(); re != nil {
		return re.SplitStr(s, limited, ToNumber(lim))
	}
	if sep.kind == KindUndefined {
		if limited && ToUint32(ToNumber(lim)) == 0 {
			return NewArrayValue([]Value{})
		}
		return NewArrayValue([]Value{StringValue(s)})
	}
	return ArrayValueOf(s.Split(ToString(sep), strNumTail(args, 1, 1)...), StringValue)
}

// stringReplace runs String.prototype.replace and replaceAll off a dynamic receiver.
// The pattern selects the machinery: a regexp box runs the regexp replacer, which walks
// matches and expands the $ substitution patterns, and anything else is a literal
// search over the code units. The replacement selects the form: a callable runs per
// match and its result is the substitution, and anything else is coerced to a string
// template. replaceAll over a regexp requires the global flag and throws a TypeError
// without it, the one rule that separates the two methods.
func stringReplace(s BStr, args []Value, all bool) Value {
	pat, rep := Arg(args, 0), Arg(args, 1)
	if re := pat.asRegExp(); re != nil {
		if rep.kind == KindFunc {
			if all {
				return StringValue(re.ReplaceAllCallStr(s, rep))
			}
			return StringValue(re.ReplaceCallStr(s, rep))
		}
		if all {
			return StringValue(re.ReplaceAllStr(s, ToString(rep)))
		}
		return StringValue(re.ReplaceStr(s, ToString(rep)))
	}
	search := ToString(pat)
	if rep.kind == KindFunc {
		return StringValue(replaceLiteralCall(s, search, rep, all))
	}
	if all {
		return StringValue(s.ReplaceAll(search, ToString(rep)))
	}
	return StringValue(s.Replace(search, ToString(rep)))
}

// replaceLiteralCall replaces a literal search string with what a replacer function
// answers for each occurrence, the form `s.replace("ab", m => ...)` takes. The function
// receives the matched text, its code-unit offset, and the whole subject, the three
// arguments the specification passes when the pattern has no capture groups. An empty
// search matches between every pair of code units and at both ends, so the walk steps
// one unit past such a match to terminate, the same advance the regexp replacer makes.
func replaceLiteralCall(s, search BStr, fn Value, all bool) BStr {
	out := BStr{}
	from, n := 0.0, s.Length()
	for {
		i := s.IndexOf(search, from)
		if i < 0 {
			break
		}
		rep := ToString(fn.Call(StringValue(search), Number(i), StringValue(s)))
		out = out.ConcatN(s.Slice(from, i), rep)
		from = i + search.Length()
		if !all {
			break
		}
		if search.Length() == 0 {
			if i >= n {
				break
			}
			out = out.ConcatN(s.Slice(i, i+1))
			from = i + 1
		}
	}
	return out.ConcatN(s.Slice(from))
}
