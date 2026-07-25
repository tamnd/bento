package value

import (
	"strings"
	"unsafe"
)

// Inspect renders a dynamic value the way the interpreter's prelude inspector
// does, the compact object-and-array spelling util.inspect and the assert message
// formatter reach for through the __bento_inspect host callee. It is not Node's
// full util.inspect (the interpreter's own inspector is not either); it is the
// same compact form both bento paths share so an AOT program and the interpreter
// agree on what an object logs as. The top-level value renders unquoted when it is
// a string, so console.log(util.inspect("hi")) reads "hi", while a string nested
// in a container is quoted the way JSON renders it.
func Inspect(v Value) BStr {
	return FromGoString(inspectValue(v, map[unsafe.Pointer]bool{}, false))
}

// inspectValue is the recursive core. nested reports whether the value sits inside
// a container, the prelude's "have we entered an object yet" test that decides
// whether a string quotes: a bare string at the top is its own text, a string held
// by an array or object is quoted. seen carries the reference values already on the
// stack so a cycle renders "[Circular]" rather than recurring forever.
func inspectValue(v Value, seen map[unsafe.Pointer]bool, nested bool) string {
	switch v.kind {
	case KindNull:
		return "null"
	case KindUndefined:
		return "undefined"
	case KindString:
		if !nested {
			return v.str().ToGoString()
		}
		return JSONStringify(v.str()).ToGoString()
	case KindNumber, KindBool, KindBigInt:
		// String(value): a bigint spells its digits with no trailing "n", the same
		// String() the prelude applies, so the console "n" suffix ConsoleValue adds
		// does not appear inside an inspected container.
		return ToString(v).ToGoString()
	case KindSymbol:
		return v.SymbolDescriptiveString().ToGoString()
	case KindFunc:
		name := v.Get(FromGoString("name"))
		if name.kind == KindString && name.str().Length() != 0 {
			return "[Function: " + name.str().ToGoString() + "]"
		}
		return "[Function (anonymous)]"
	}

	// The reference kinds. A value already on the stack is a cycle.
	if v.ref != nil {
		if seen[v.ref] {
			return "[Circular]"
		}
		seen[v.ref] = true
		defer delete(seen, v.ref)
	}

	switch v.kind {
	case KindArray:
		o := v.object()
		if len(o.elems) == 0 {
			return "[]"
		}
		parts := make([]string, len(o.elems))
		for i, e := range o.elems {
			// A hole contributes the empty string the way Array.prototype.map leaves a
			// hole and join renders it, so [ , 2 ] reads "[ , 2 ]".
			if isHole(e) {
				parts[i] = ""
				continue
			}
			parts[i] = inspectValue(e, seen, true)
		}
		return "[ " + strings.Join(parts, ", ") + " ]"
	default:
		// A plain object, or a boxed value that reads as one (an error boxes to an
		// object; its enumerable keys render here since the runtime carries no
		// Error brand the dynamic layer can test, a deferred fidelity gap).
		keys := v.OwnEnumerableKeys().elems
		if len(keys) == 0 {
			return "{}"
		}
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k.ToGoString() + ": " + inspectValue(v.Get(k), seen, true)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	}
}
