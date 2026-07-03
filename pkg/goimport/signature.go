package goimport

import (
	"go/types"
	"strings"
)

// This file exposes a go: function's parameter and result types to the lowerer,
// which the generated .d.ts alone cannot carry. Go's int, int64, and float64 all
// project to the TypeScript number (section 6.2), so the checker types a call the
// same whichever the Go signature wants, and the lowerer would not know whether to
// emit a plain float64, an int conversion, or the 64-bit range check of section
// 7.5. Signatures reads the real Go signature and hands the lowerer the Go type
// keyword per parameter and result, which is exactly the distinction the number
// marshaling turns on.

// FuncSig describes one exported function's boundary crossings by the Go type
// keyword of each parameter and each non-error result: "string", "bool", "int",
// "int64", "float64", and the rest of the numeric basics. OK reports whether the
// whole signature is in a shape the lowerer marshals today; an unclassifiable
// parameter or result, more than one non-error result, or an error in a non-trailing
// position clears it, so the lowerer hands the call back rather than emit an unsound
// crossing. A cleared OK still carries whatever it classified, for a diagnostic, but
// the lowerer reads only the flag.
//
// Variadic reports whether the function ends in a ...T rest parameter (section 6.9).
// When it is set the trailing entry of Params, ParamConv, and ParamElem describes the
// element type T rather than the []T the parameter is at the Go level, so the lowerer
// marshals each spread argument as a single T and passes them positionally into the
// Go variadic call. A variadic whose element is itself a supported crossing keeps OK
// set; only an unclassifiable element clears it.
//
// Throws reports whether the signature ends in a trailing error, the (T, error)
// idiom that projects to a value that throws on failure (section 6.6). The error
// is dropped from Results, so Results holds only the non-error results the call
// returns; the lowerer wraps the call in the throw bridge when Throws is set.
//
// ParamConv is parallel to Params: for a parameter that is a defined type over a
// basic (time.Duration over int64), it names the type a bento value converts to at
// the call so the Go function receives a real time.Duration and not a bare int64
// (section 6.11); for a plain basic parameter it is the zero DefinedConv. Results
// carries only the underlying keyword of a defined-type result, and ResultDefined
// records that the result is a defined type so the lowerer strips its brand before
// marshaling it back.
//
// ParamElem and ResultElem are parallel to Params and Results: for a parameter or
// result that is a slice of a basic ([]string, []float64) the keyword is "slice" and
// the element keyword rides here, so the lowerer marshals the array element by
// element (section 6.4); for a scalar crossing the element is the empty string. A
// []byte is not a slice crossing: it carries the keyword "bytes" with an empty
// element and marshals as one Uint8Array through the byte bridge (section 7.3),
// because the buffer crosses as a whole and shares its backing storage rather than
// element by element.
//
// An opaque handle (section 6.13), a foreign named type the bridge does not project
// (a struct with no exported fields or methods, or a named func type), carries the
// keyword "opaque" in Params or Results with its import path and Go name packed into
// the parallel element slot by OpaqueElem. The value crosses by identity: bento
// holds the real Go value as a token and hands it back to another go: call without
// ever inspecting it, so the lowerer emits no conversion and only names the foreign
// Go type where the guard closure needs it.
//
// A Go any (the alias for interface{}) carries the keyword "any" in Params or Results
// with an empty element slot, the dynamic crossing of section 6.12. It projects to
// unknown and crosses as a boxed bento value, so the lowerer boxes the argument to a
// value.Value on the way in and unboxes the result on the way out.
type FuncSig struct {
	Params        []string
	ParamConv     []DefinedConv
	ParamElem     []string
	Results       []string
	ResultElem    []string
	ResultDefined bool
	Throws        bool
	Variadic      bool
	OK            bool
}

// DefinedConv names the conversion a defined-type parameter takes: Name is the
// defined type's name and Path the import path it comes from, so the emitted call
// converts a bento value to the named type qualified by that package. An empty Name
// marks a plain basic parameter that needs no defined-type conversion.
type DefinedConv struct {
	Name string
	Path string
}

// Signatures loads the Go package at importPath and returns the signature of each
// exported top-level function, keyed by name, for the lowerer to marshal a go:
// call against. It reuses the same go/packages load the declaration generator runs
// (section 5.1), so it sees the genuine go/types view of every signature. A method
// (a function with a receiver) is not a top-level binding a go: import names, so it
// is skipped.
func Signatures(importPath string) (map[string]FuncSig, error) {
	pkg, err := loadPackage(importPath)
	if err != nil {
		return nil, err
	}
	out := map[string]FuncSig{}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Recv() != nil {
			continue
		}
		out[name] = classifySignature(sig)
	}
	return out, nil
}

// classifySignature reduces a Go signature to the marshal keywords the lowerer
// reads. It clears OK for anything outside the crossings this slice covers: a
// parameter or non-error result that is not a plain basic type, more than one
// non-error result, or an error in a non-trailing position. A trailing error is the
// (T, error) throw idiom (section 6.6): it is dropped from Results, sets Throws, and
// leaves OK intact so the lowerer wraps the call in the throw bridge. A variadic tail
// is classified by its element type and flagged (section 6.9), so the lowerer spreads
// each argument as a single element into the Go call. The keywords are filled
// best-effort so a diagnostic can name a crossing even when OK is clear.
func classifySignature(sig *types.Signature) FuncSig {
	ok := true
	variadic := sig.Variadic()
	np := sig.Params().Len()
	params := make([]string, 0, np)
	convs := make([]DefinedConv, 0, np)
	paramElems := make([]string, 0, np)
	for i := 0; i < np; i++ {
		t := sig.Params().At(i).Type()
		if variadic && i == np-1 {
			// The trailing parameter of a variadic function is a []T the caller spreads
			// individual T arguments into. Classify the element T so each spread argument
			// marshals as one T; the lowerer passes them positionally into the Go call,
			// which reassembles the slice (section 6.9).
			s, good := t.(*types.Slice)
			if !good {
				ok = false
				params = append(params, "")
				convs = append(convs, DefinedConv{})
				paramElems = append(paramElems, "")
				continue
			}
			t = s.Elem()
		}
		kw, conv, elem, good := classifyParamType(t)
		if !good {
			ok = false
		}
		params = append(params, kw)
		convs = append(convs, conv)
		paramElems = append(paramElems, elem)
	}
	var results []string
	var resultElems []string
	throws := false
	resultDefined := false
	n := sig.Results().Len()
	for i := 0; i < n; i++ {
		t := sig.Results().At(i).Type()
		if isErrorType(t) {
			// A trailing error hoists to a throw; an error anywhere else is not the
			// idiom and has no marshaling, so the call hands back.
			if i == n-1 {
				throws = true
			} else {
				ok = false
			}
			continue
		}
		if bytesCrossing(t) {
			results = append(results, "bytes")
			resultElems = append(resultElems, "")
			continue
		}
		if elem, good := sliceCrossing(t); good {
			results = append(results, "slice")
			resultElems = append(resultElems, elem)
			continue
		}
		if key, val, good := mapCrossing(t); good {
			results = append(results, "map")
			resultElems = append(resultElems, MapElem(key, val))
			continue
		}
		if path, name, fields, good := structCrossing(t); good {
			results = append(results, "struct")
			resultElems = append(resultElems, StructElem(path, name, fields))
			continue
		}
		if anyCrossing(t) {
			results = append(results, "any")
			resultElems = append(resultElems, "")
			continue
		}
		if path, name, good := opaqueCrossing(t); good {
			results = append(results, "opaque")
			resultElems = append(resultElems, OpaqueElem(path, name))
			continue
		}
		kw, conv, good := crossingType(t)
		if !good {
			ok = false
		}
		if conv.Name != "" {
			resultDefined = true
		}
		results = append(results, kw)
		resultElems = append(resultElems, "")
	}
	if len(results) > 1 {
		// More than one non-error result maps to a tuple, a later slice; only a single
		// value (with or without a hoisted error) is marshaled today.
		ok = false
	}
	return FuncSig{Params: params, ParamConv: convs, ParamElem: paramElems, Results: results, ResultElem: resultElems, ResultDefined: resultDefined, Throws: throws, Variadic: variadic, OK: ok}
}

// classifyParamType classifies one parameter's Go type into the marshal keyword, the
// defined-type conversion, and the element keyword the lowerer reads, reporting
// whether the crossing is one this slice marshals. It is shared by the fixed
// parameters and, for a variadic function, by the element type of the trailing
// parameter, so a variadic ...string classifies its element exactly as a plain
// string parameter would (section 6.9). The order matches the loop it replaces: a
// slice of a basic first, then any, then an opaque handle, then a scalar crossing.
func classifyParamType(t types.Type) (kw string, conv DefinedConv, elem string, ok bool) {
	if bytesCrossing(t) {
		return "bytes", DefinedConv{}, "", true
	}
	if e, good := sliceCrossing(t); good {
		return "slice", DefinedConv{}, e, true
	}
	if key, val, good := mapCrossing(t); good {
		return "map", DefinedConv{}, MapElem(key, val), true
	}
	if anyCrossing(t) {
		return "any", DefinedConv{}, "", true
	}
	if path, name, good := opaqueCrossing(t); good {
		return "opaque", DefinedConv{}, OpaqueElem(path, name), true
	}
	kw, conv, good := crossingType(t)
	return kw, conv, "", good
}

// crossingType classifies a parameter or result type for the boundary: a plain
// basic returns its keyword and a zero DefinedConv, and a defined type over a basic
// (time.Duration over int64) returns the underlying basic's keyword and the
// DefinedConv the lowerer converts through, so the branded projection of section
// 6.11 crosses as its underlying value while the call still passes the real named
// type. Anything else (a struct, an interface, an unexported or non-basic defined
// type) returns "" and false so the signature hands back. The defined type must be
// exported and belong to a package, because the .d.ts projects only exported types
// and the emitted conversion qualifies the name by its package.
func crossingType(t types.Type) (string, DefinedConv, bool) {
	if kw := basicKeyword(t); kw != "" {
		return kw, DefinedConv{}, true
	}
	named, ok := t.(*types.Named)
	if !ok {
		return "", DefinedConv{}, false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || !obj.Exported() {
		return "", DefinedConv{}, false
	}
	kw := basicKeyword(named.Underlying())
	if kw == "" {
		return "", DefinedConv{}, false
	}
	return kw, DefinedConv{Name: obj.Name(), Path: obj.Pkg().Path()}, true
}

// bytesCrossing reports whether t is a []byte, the one slice that crosses whole as a
// Uint8Array rather than element by element (section 7.3). A []byte projects to a
// Uint8Array and its backing storage is the buffer's own bytes, so the lowerer
// marshals it through the byte bridge in one move; every other slice of a basic goes
// through sliceCrossing. It matches an unnamed slice of uint8, which is what a []byte
// or []uint8 parameter is, the same shape sliceCrossing keys on for its slices.
func bytesCrossing(t types.Type) bool {
	s, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	b, ok := s.Elem().(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// sliceCrossing classifies a slice of a plain basic for the boundary, returning the
// element's Go type keyword and true so the lowerer marshals a []string or []float64
// element by element as a bento array (section 6.4). A []byte is not a slice
// crossing: bytesCrossing claims it first because it crosses whole as a Uint8Array
// (section 7.3), so the uint8 element is rejected here as a defense in case this is
// reached on its own. A slice of a defined type or a non-basic element also returns
// false, because only the scalar element crossings are covered here, so those await
// their own slice.
func sliceCrossing(t types.Type) (string, bool) {
	s, ok := t.(*types.Slice)
	if !ok {
		return "", false
	}
	kw := basicKeyword(s.Elem())
	if kw == "" || kw == "uint8" {
		return "", false
	}
	return kw, true
}

// mapCrossing classifies a Go map of a plain basic key to a plain basic value for
// the boundary, returning the key and value Go type keywords and true so the lowerer
// marshals it entry by entry as a bento Map (section 6.5). Both the key and the value
// must be a plain basic this slice crosses: the key becomes a bento number, string,
// or boolean, which are exactly the key kinds the value model's Map constructors
// build (a numeric key maps to number, so map[int]V and map[float64]V share one
// numbered key kind), and the value takes the same scalar crossing a single value
// would. A map whose key or value is a defined type, a slice, another map, or any
// other non-basic shape returns false, so the call hands back rather than cross a
// shape this slice does not marshal; those await their own slices.
func mapCrossing(t types.Type) (string, string, bool) {
	m, ok := t.(*types.Map)
	if !ok {
		return "", "", false
	}
	key := basicKeyword(m.Key())
	if key == "" {
		return "", "", false
	}
	val := basicKeyword(m.Elem())
	if val == "" {
		return "", "", false
	}
	return key, val, true
}

// StructField is one exported field of a struct crossing: its Go field name and the
// Go type keyword its value marshals by, the pair the lowerer reads to build the
// interned struct field and the per-field crossing (section 6.7).
type StructField struct {
	Name    string
	Keyword string
}

// structCrossing classifies a Go named struct with exported basic fields as a struct
// result crossing, returning its import path, Go name, and the exported fields and
// true so the lowerer marshals it to the object box the interface projection shares
// (sections 6.7, 7.4). It fires only for a struct this slice can read whole: a named,
// exported type whose underlying is a struct with at least one exported field, every
// exported field of a plain basic type. Unexported fields are ignored, because the
// author cannot read them and the projection does not surface them. A struct with no
// exported field is not a struct crossing but an opaque token (opaqueCrossing claims
// it), and an exported field of a composite type (a nested struct, a slice, another
// map) clears the crossing so the call hands back rather than marshal a field this
// slice cannot cross; those await their own slices. Methods are allowed and ignored:
// this slice reads a struct's data, and calling its methods is a later slice, so a
// struct with methods still crosses as the data an author reads.
func structCrossing(t types.Type) (string, string, []StructField, bool) {
	named, ok := t.(*types.Named)
	if !ok {
		return "", "", nil, false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || !obj.Exported() {
		return "", "", nil, false
	}
	if isErrorType(named) {
		return "", "", nil, false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", "", nil, false
	}
	var fields []StructField
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}
		kw := basicKeyword(f.Type())
		if kw == "" {
			// An exported field of a composite type has no scalar crossing here, so the
			// whole struct hands back until its own slice marshals that field.
			return "", "", nil, false
		}
		fields = append(fields, StructField{Name: f.Name(), Keyword: kw})
	}
	if len(fields) == 0 {
		// A struct with no exported field is a pure token, left to opaqueCrossing.
		return "", "", nil, false
	}
	return obj.Pkg().Path(), obj.Name(), fields, true
}

// StructElem packs a struct crossing's import path, Go type name, and exported fields
// into the single element string it rides in ResultElem, so the lowerer builds the
// interned struct and the per-field crossings without a second parallel slice. The
// path and name lead, then one entry per field spelled name:keyword, all joined by a
// NUL, which never appears in a Go import path, identifier, or type keyword, so the
// split is exact. A field name and its keyword are joined by a colon, which a Go
// identifier and a type keyword never contain, so that split is exact too.
func StructElem(path, name string, fields []StructField) string {
	parts := make([]string, 0, len(fields)+2)
	parts = append(parts, path, name)
	for _, f := range fields {
		parts = append(parts, f.Name+":"+f.Keyword)
	}
	return strings.Join(parts, "\x00")
}

// SplitStructElem recovers the import path, Go type name, and exported fields
// StructElem packed, for the lowerer to build the interned struct reference and the
// per-field crossings.
func SplitStructElem(elem string) (string, string, []StructField) {
	parts := strings.Split(elem, "\x00")
	if len(parts) < 2 {
		return "", "", nil
	}
	fields := make([]StructField, 0, len(parts)-2)
	for _, p := range parts[2:] {
		name, kw, _ := strings.Cut(p, ":")
		fields = append(fields, StructField{Name: name, Keyword: kw})
	}
	return parts[0], parts[1], fields
}

// opaqueCrossing classifies a foreign named type the bridge does not project as an
// opaque handle, returning its import path and Go name and true so the lowerer holds
// it as a token and hands it back untouched (section 6.13). It fires only for the
// types that unambiguously have no richer projection: an exported named type whose
// underlying is a struct with no exported fields and whose method set is empty, or a
// named func type with no methods. Those are the option-value and hidden-concrete
// shapes an author receives from one call and passes to another. Everything with a
// faithful projection returns false so its own slice marshals it: error and the
// well-known interfaces have named helpers, a defined type over a basic is the
// branded number of section 6.11, an empty interface is any (section 6.12), and a
// struct with exported fields or methods is a class (section 6.7). Being conservative
// keeps the crossing sound: a type this does not claim hands the call back rather
// than cross a shape bento cannot honor.
func opaqueCrossing(t types.Type) (string, string, bool) {
	named, ok := t.(*types.Named)
	if !ok {
		return "", "", false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || !obj.Exported() {
		return "", "", false
	}
	if isErrorType(named) {
		return "", "", false
	}
	// A well-known interface (io.Reader, context.Context) has a vocabulary projection,
	// not an opaque token, so it is left to that helper.
	short := obj.Pkg().Name() + "." + obj.Name()
	qualified := obj.Pkg().Path() + "." + obj.Name()
	if _, known := wellKnown[short]; known {
		return "", "", false
	}
	if _, known := wellKnown[qualified]; known {
		return "", "", false
	}
	// A method on the named type means the author can call into it, which is a class or
	// interface projection, not an opaque handle.
	if named.NumMethods() > 0 {
		return "", "", false
	}
	switch u := named.Underlying().(type) {
	case *types.Struct:
		// A struct with an exported field is a shape the author reads, a class (section
		// 6.7); only a field-free struct is a pure token.
		if hasExportedField(u) {
			return "", "", false
		}
	case *types.Signature:
		// A named func type carries a callback the bento side never calls itself; it holds
		// it and hands it back, the option-value shape.
	default:
		// A basic underlying is the branded number of section 6.11, an interface is any or
		// a method set (sections 6.12, 6.8), and a slice, map, array, or channel has its
		// own structural projection, so none of them is an opaque token.
		return "", "", false
	}
	return obj.Pkg().Path(), obj.Name(), true
}

// hasExportedField reports whether a struct type has any exported field, the mark
// that separates a class projection (a shape the author reads) from an opaque token.
func hasExportedField(st *types.Struct) bool {
	for i := 0; i < st.NumFields(); i++ {
		if st.Field(i).Exported() {
			return true
		}
	}
	return false
}

// anyCrossing reports whether a parameter or result is a Go any (the alias for
// interface{}), the dynamic crossing of section 6.12. It fires for an interface with an
// empty method set, which is exactly interface{}, its any alias, and a named empty
// interface, all of which hold any value and project to unknown. error and every other
// named interface carry a method set, so they are left to their own projection (a throw
// for error, a method interface for the rest, sections 6.6 and 6.8), and a struct, a
// basic, or a slice is not an interface at all and never reaches here.
func anyCrossing(t types.Type) bool {
	it, ok := t.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	return it.Empty()
}

// OpaqueElem packs a foreign type's import path and Go name into the single element
// string an opaque crossing rides in ParamElem or ResultElem, so the lowerer can name
// the type in the guard closure without a second parallel slice. The separator is a
// NUL, which never appears in a Go import path or identifier, so the split is exact.
func OpaqueElem(path, name string) string {
	return path + "\x00" + name
}

// SplitOpaqueElem recovers the import path and Go name OpaqueElem packed, for the
// lowerer to build the qualified type reference.
func SplitOpaqueElem(elem string) (string, string) {
	path, name, _ := strings.Cut(elem, "\x00")
	return path, name
}

// MapElem packs a map crossing's key and value Go type keywords into the single
// element string it rides in ParamElem or ResultElem, so the lowerer reads both the
// key kind that picks the bento Map constructor and the value kind that marshals each
// value without a second parallel slice. The separator is a NUL, which never appears
// in a Go type keyword, so the split is exact, the same packing an opaque crossing
// uses for its path and name.
func MapElem(key, val string) string {
	return key + "\x00" + val
}

// SplitMapElem recovers the key and value Go type keywords MapElem packed, for the
// lowerer to build the bento Map constructor and the per-entry crossings.
func SplitMapElem(elem string) (string, string) {
	key, val, _ := strings.Cut(elem, "\x00")
	return key, val
}

// basicKeyword returns the Go type keyword for a plain basic type, and "" for
// anything else. It matches on the unnamed basic types only: a defined type over a
// basic (time.Duration over int64) projects to a branded alias in the .d.ts
// (section 6.11), not number, so its crossing differs and it is deliberately not
// classified here. byte and rune are basics whose kind is uint8 and int32, so they
// classify as those, which is the right conversion target.
func basicKeyword(t types.Type) string {
	b, ok := t.(*types.Basic)
	if !ok {
		return ""
	}
	switch b.Kind() {
	case types.String:
		return "string"
	case types.Bool:
		return "bool"
	case types.Int:
		return "int"
	case types.Int8:
		return "int8"
	case types.Int16:
		return "int16"
	case types.Int32:
		return "int32"
	case types.Int64:
		return "int64"
	case types.Uint:
		return "uint"
	case types.Uint8:
		return "uint8"
	case types.Uint16:
		return "uint16"
	case types.Uint32:
		return "uint32"
	case types.Uint64:
		return "uint64"
	case types.Uintptr:
		return "uintptr"
	case types.Float32:
		return "float32"
	case types.Float64:
		return "float64"
	default:
		// complex64, complex128, unsafe.Pointer, and the untyped kinds have no clean
		// number crossing, so they are unsupported (section 6.14).
		return ""
	}
}

// isErrorType reports whether t is the built-in error interface, the trailing
// result the throw bridge keys on (section 6.6). It matches the same way the type
// mapper does, by the interface's identity as the universe's error.
func isErrorType(t types.Type) bool {
	n, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg() == nil && n.Obj().Name() == "error"
}
