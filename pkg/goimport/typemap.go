package goimport

import (
	"go/types"
	"strconv"
	"strings"
)

// This file is the reference table of document 16 section 6 turned into code: it
// projects a Go type into the TypeScript type the generated .d.ts declares. It is
// a pure function of go/types, so the same mapping serves the declaration
// generator (section 5) and, later, the marshaling that lowering emits (section
// 7). Where a Go type has an obvious TypeScript twin the twin is used; where it
// does not, a named helper from the bento:go vocabulary module is named rather
// than an approximation invented, so a reader of the .d.ts sees a real type or a
// documented handle, never a lie.

// Helper is a type name exported by the bento:go vocabulary module. The generator
// collects the set a declaration file references so it can emit one import for
// exactly the helpers used and nothing more (section 5.2).
type Helper string

const (
	// HelperError is GoError, the thrown/handled projection of a Go error value.
	HelperError Helper = "GoError"
	// HelperChannel is GoChannel<T>, a Go channel as an async iterable (section 8).
	HelperChannel Helper = "GoChannel"
	// HelperContext is GoContext, an explicit Go context handle (section 10.2).
	HelperContext Helper = "GoContext"
	// HelperOpaque is GoOpaque<"pkg.Name">, a token for a type the bridge does not
	// project but that still passes through Go call to Go call (section 6.13).
	HelperOpaque Helper = "GoOpaque"
	// HelperUnsupported is GoUnsupported, the marker for a type that cannot cross
	// at all, so using the symbol is a legible checker error (section 6.14).
	HelperUnsupported Helper = "GoUnsupported"
	// HelperReader, HelperWriter, and the rest are the named projections of the
	// common standard-library io interfaces (section 6.8).
	HelperReader      Helper = "GoReader"
	HelperWriter      Helper = "GoWriter"
	HelperCloser      Helper = "GoCloser"
	HelperReadCloser  Helper = "GoReadCloser"
	HelperWriteCloser Helper = "GoWriteCloser"
	HelperReadWriter  Helper = "GoReadWriter"
)

// wellKnown maps a fully qualified Go type ("path.Name") to the bento:go helper
// that projects it, so a signature that mentions io.Reader reads as GoReader and
// a context.Context reads as GoContext instead of an opaque handle. These are the
// projections section 6.8 and section 10 promise the vocabulary module ships.
var wellKnown = map[string]Helper{
	"io.Reader":       HelperReader,
	"io.Writer":       HelperWriter,
	"io.Closer":       HelperCloser,
	"io.ReadCloser":   HelperReadCloser,
	"io.WriteCloser":  HelperWriteCloser,
	"io.ReadWriter":   HelperReadWriter,
	"context.Context": HelperContext,
}

// Mapper projects Go types for one package's generated declaration file. It is
// created per package because a named type defined in the package being generated
// is referenced by its bare name (it gets its own declaration alongside), while a
// named type from another package is a foreign token projected as an opaque handle
// (section 6.13). The used set records every helper a projection named so the
// emitter can build the bento:go import.
type Mapper struct {
	// pkg is the package whose declarations are being generated. A named type whose
	// Obj().Pkg() is this package is "local" and referenced by name; anything else
	// is foreign. It may be nil, in which case every named type is treated foreign,
	// which is the safe over-approximation.
	pkg  *types.Package
	used map[Helper]bool
}

// NewMapper builds a Mapper for the package being generated. Pass the same
// *types.Package the declaration walk iterates so local type references resolve to
// bare names rather than opaque handles.
func NewMapper(pkg *types.Package) *Mapper {
	return &Mapper{pkg: pkg, used: map[Helper]bool{}}
}

// Used reports the bento:go helpers every Map call so far has named, so the
// emitter imports exactly the vocabulary the file uses (section 5.2).
func (m *Mapper) Used() []Helper {
	out := make([]Helper, 0, len(m.used))
	// Emit in the fixed order the constants are declared so the generated import is
	// stable across runs, which keeps the .d.ts cache key meaningful (section 4.5).
	for _, h := range helperOrder {
		if m.used[h] {
			out = append(out, h)
		}
	}
	return out
}

var helperOrder = []Helper{
	HelperError, HelperChannel, HelperContext, HelperOpaque, HelperUnsupported,
	HelperReader, HelperWriter, HelperCloser, HelperReadCloser, HelperWriteCloser,
	HelperReadWriter,
}

func (m *Mapper) mark(h Helper) { m.used[h] = true }

// Map projects a Go type into its TypeScript type text. It walks the type
// structure of section 6: primitives map to their twins, []byte to Uint8Array, a
// pointer is transparent (a *T projects as T since the object box aliases the
// pointee, section 7.4), a channel to GoChannel, a func to a function type, a
// named type to its projection, and anything with no faithful mapping to a
// GoUnsupported marker so the failure is a checker error at the call site.
func (m *Mapper) Map(t types.Type) string {
	switch u := t.(type) {
	case *types.Basic:
		return m.mapBasic(u)
	case *types.Pointer:
		// A Go pointer is transparent in the projection: *Decoder is a Decoder,
		// because the object box holds the pointer and dispatches methods through it
		// (section 7.4). Nil-ness is a runtime concern the marshaling handles, not a
		// shape the type needs to carry.
		return m.Map(u.Elem())
	case *types.Slice:
		if isByte(u.Elem()) {
			return "Uint8Array"
		}
		return m.Map(u.Elem()) + "[]"
	case *types.Array:
		// A fixed array is a value type, so it always copies (section 6.4), but its
		// shape from TypeScript is still a plain array of the element type.
		return m.Map(u.Elem()) + "[]"
	case *types.Map:
		return "Map<" + m.Map(u.Key()) + ", " + m.Map(u.Elem()) + ">"
	case *types.Chan:
		m.mark(HelperChannel)
		return "GoChannel<" + m.Map(u.Elem()) + ">"
	case *types.Signature:
		return m.mapSignature(u)
	case *types.Named:
		return m.mapNamed(u)
	case *types.Interface:
		// A bare interface literal in a signature: empty is any (unknown), a method
		// set with no name has no TypeScript twin to reference.
		if u.Empty() {
			return "unknown"
		}
		m.mark(HelperUnsupported)
		return "GoUnsupported"
	case *types.TypeParam:
		// A generic type parameter carries over by its own name (section 11).
		return u.Obj().Name()
	case *types.Struct:
		// An anonymous struct has no name to declare, so it crosses as an opaque
		// token rather than a fabricated shape.
		m.mark(HelperOpaque)
		return "GoOpaque<\"struct\">"
	default:
		m.mark(HelperUnsupported)
		return "GoUnsupported"
	}
}

// mapBasic projects a predeclared type. The integer story is section 6.2: every
// sized integer projects to number by default, including int64/uint64, with the
// range check living in the marshaling rather than the type. uintptr, the complex
// kinds, and unsafe.Pointer have no safe TypeScript meaning and project as
// GoUnsupported (section 6.14).
func (m *Mapper) mapBasic(b *types.Basic) string {
	switch b.Kind() {
	case types.Bool, types.UntypedBool:
		return "boolean"
	case types.String, types.UntypedString:
		return "string"
	case types.Float32, types.Float64, types.UntypedFloat,
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
		types.UntypedInt, types.UntypedRune:
		return "number"
	case types.Uintptr, types.UnsafePointer,
		types.Complex64, types.Complex128, types.UntypedComplex:
		m.mark(HelperUnsupported)
		return "GoUnsupported"
	default:
		m.mark(HelperUnsupported)
		return "GoUnsupported"
	}
}

// mapSignature projects a func type as a TypeScript function type. The result
// follows the (T, error) rule of section 6.6 in throw mode: a lone error result
// is dropped and becomes a throw, a single non-error result is that type, and
// multiple results are a tuple.
func (m *Mapper) mapSignature(sig *types.Signature) string {
	return "(" + m.ParamList(sig) + ") => " + m.mapResults(sig.Results())
}

// ParamList renders a signature's parameters as a TypeScript parameter list
// without the enclosing parentheses, so it is shared by func-type projection and
// by the declaration emitter for top-level functions and methods. Parameters
// carry across positionally, an unnamed or blank parameter gets a positional name,
// and a variadic tail becomes a rest parameter over the element type.
func (m *Mapper) ParamList(sig *types.Signature) string {
	var b strings.Builder
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		p := params.At(i)
		name := tsParamName(p.Name(), i)
		if sig.Variadic() && i == params.Len()-1 {
			// A Go variadic ...T is a slice at the type level; the rest parameter
			// wants the element type spread, so unwrap the slice.
			b.WriteString("..." + name + ": ")
			if s, ok := p.Type().(*types.Slice); ok {
				b.WriteString(m.Map(s.Elem()) + "[]")
			} else {
				b.WriteString(m.Map(p.Type()) + "[]")
			}
			continue
		}
		b.WriteString(name + ": " + m.Map(p.Type()))
	}
	return b.String()
}

// tsParamName maps a Go parameter name to one legal in a TypeScript parameter
// position. A parameter name in a .d.ts is only a label the type rides on, never
// referenced by a caller, so a blank, underscore, or reserved-word name (Go's
// `new`, `function`, and the rest are legal Go identifiers but reserved in
// TypeScript) is replaced by a positional name rather than emitted verbatim, which
// would make the whole declaration fail to parse.
func tsParamName(name string, i int) string {
	if name == "" || name == "_" || tsReserved[name] {
		return "a" + strconv.Itoa(i)
	}
	return name
}

// tsReserved is the set of ECMAScript reserved words that cannot appear as a
// binding name, so a Go parameter that happens to spell one is renamed. The
// contextual keywords TypeScript allows as identifiers (such as `of` and `type`)
// are deliberately left out, since they parse as parameter names and keeping the
// original reads better.
var tsReserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "enum": true, "export": true, "extends": true, "false": true,
	"finally": true, "for": true, "function": true, "if": true, "import": true,
	"in": true, "instanceof": true, "new": true, "null": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, "void": true, "while": true,
	"with": true,
}

// Results projects a signature's result tuple in throw mode, exposed so the
// declaration emitter shares the exact (T, error) hoisting the func-type
// projection uses (section 6.6).
func (m *Mapper) Results(sig *types.Signature) string { return m.mapResults(sig.Results()) }

// TypeParams renders a type parameter list as TypeScript type parameters, so a
// generic function or type projects with the same parameters it declares in Go
// (section 11). A constraint that is a well-known interface projects as an
// extends bound; anything else is dropped, because the Go compiler enforces the
// real constraint at instantiation (section 11.3). It returns "" for a
// non-generic declaration.
func (m *Mapper) TypeParams(tp *types.TypeParamList) string {
	if tp == nil || tp.Len() == 0 {
		return ""
	}
	parts := make([]string, tp.Len())
	for i := 0; i < tp.Len(); i++ {
		p := tp.At(i)
		name := p.Obj().Name()
		if bound := m.constraintBound(p); bound != "" {
			name += " extends " + bound
		}
		parts[i] = name
	}
	return "<" + strings.Join(parts, ", ") + ">"
}

// constraintBound returns the TypeScript extends bound for a type parameter's
// constraint when it is a named interface the bridge projects (a method-set
// constraint like io.Reader becomes GoReader), or "" when the constraint has no
// TypeScript meaning and is safely dropped (comparable, any, a type-set union).
func (m *Mapper) constraintBound(p *types.TypeParam) string {
	c := p.Constraint()
	if c == nil {
		return ""
	}
	named, ok := c.(*types.Named)
	if !ok {
		return ""
	}
	iface, ok := named.Underlying().(*types.Interface)
	if !ok || iface.Empty() {
		return ""
	}
	// Only project the constraint when it resolves to a named helper; a type set
	// or a bare method interface has no faithful TypeScript bound (section 11.3).
	if obj := named.Obj(); obj.Pkg() != nil {
		short := obj.Pkg().Name() + "." + obj.Name()
		if h, ok := wellKnown[short]; ok {
			m.mark(h)
			return string(h)
		}
	}
	return ""
}

// mapResults projects a result tuple in throw mode (section 6.6). It is shared by
// function types here and by the declaration emitter for top-level functions and
// methods, so the (T, error) hoisting is defined in exactly one place.
func (m *Mapper) mapResults(res *types.Tuple) string {
	// Split the trailing error, if any, off the result list: in throw mode it
	// leaves the signature and becomes a thrown GoError.
	n := res.Len()
	hasErr := n > 0 && isError(res.At(n-1).Type())
	if hasErr {
		n--
	}
	switch n {
	case 0:
		return "void"
	case 1:
		return m.Map(res.At(0).Type())
	default:
		parts := make([]string, n)
		for i := 0; i < n; i++ {
			parts[i] = m.Map(res.At(i).Type())
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
}

// mapNamed projects a named (defined) type. error is GoError, a well-known
// standard-library interface is its named helper, a type defined in the package
// being generated is referenced by its bare name (with type arguments when it is a
// generic instantiation), and any other foreign named type is an opaque handle
// (section 6.13). A named type whose underlying type is the empty interface is
// any, so a package's own alias for interface{} still reads as unknown.
func (m *Mapper) mapNamed(n *types.Named) string {
	obj := n.Obj()
	if isError(n) {
		m.mark(HelperError)
		return "GoError"
	}
	if obj.Pkg() != nil {
		qualified := obj.Pkg().Path() + "." + obj.Name()
		short := obj.Pkg().Name() + "." + obj.Name()
		if h, ok := wellKnown[short]; ok {
			m.mark(h)
			return string(h)
		}
		if h, ok := wellKnown[qualified]; ok {
			m.mark(h)
			return string(h)
		}
	}
	if iface, ok := n.Underlying().(*types.Interface); ok && iface.Empty() {
		return "unknown"
	}
	if m.isLocal(obj) {
		return m.localRef(n)
	}
	// A foreign named type the bridge does not project crosses as an opaque token
	// tagged with its Go name, which is exactly how an option-value API is meant to
	// be used (section 6.13).
	m.mark(HelperOpaque)
	tag := obj.Name()
	if obj.Pkg() != nil {
		tag = obj.Pkg().Name() + "." + obj.Name()
	}
	return "GoOpaque<\"" + tag + "\">"
}

// localRef references a type declared in the package being generated by its bare
// name, adding the TypeScript type arguments when the reference is a generic
// instantiation like List[int].
func (m *Mapper) localRef(n *types.Named) string {
	name := n.Obj().Name()
	args := n.TypeArgs()
	if args == nil || args.Len() == 0 {
		return name
	}
	parts := make([]string, args.Len())
	for i := 0; i < args.Len(); i++ {
		parts[i] = m.Map(args.At(i))
	}
	return name + "<" + strings.Join(parts, ", ") + ">"
}

func (m *Mapper) isLocal(obj types.Object) bool {
	return m.pkg != nil && obj.Pkg() == m.pkg
}

// isByte reports whether t is byte (uint8), the element type that makes a slice
// project to Uint8Array rather than number[] (section 6.3).
func isByte(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// isError reports whether t is the predeclared error interface, the result the
// (T, error) idiom hoists to a throw (section 6.6).
func isError(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg() == nil && n.Obj().Name() == "error"
}
