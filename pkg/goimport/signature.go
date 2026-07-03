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
// whole signature is in a shape the lowerer marshals today; a variadic, an
// unclassifiable parameter or result, more than one non-error result, or an error
// in a non-trailing position clears it, so the lowerer hands the call back rather
// than emit an unsound crossing. A cleared OK still carries whatever it classified,
// for a diagnostic, but the lowerer reads only the flag.
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
// []byte is deliberately not a slice crossing, because it projects to a Uint8Array
// (section 7.3), a later slice, so it clears OK and hands the call back.
//
// An opaque handle (section 6.13), a foreign named type the bridge does not project
// (a struct with no exported fields or methods, or a named func type), carries the
// keyword "opaque" in Params or Results with its import path and Go name packed into
// the parallel element slot by OpaqueElem. The value crosses by identity: bento
// holds the real Go value as a token and hands it back to another go: call without
// ever inspecting it, so the lowerer emits no conversion and only names the foreign
// Go type where the guard closure needs it.
type FuncSig struct {
	Params        []string
	ParamConv     []DefinedConv
	ParamElem     []string
	Results       []string
	ResultElem    []string
	ResultDefined bool
	Throws        bool
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
// variadic tail, a parameter or non-error result that is not a plain basic type,
// more than one non-error result, or an error in a non-trailing position. A
// trailing error is the (T, error) throw idiom (section 6.6): it is dropped from
// Results, sets Throws, and leaves OK intact so the lowerer wraps the call in the
// throw bridge. The keywords are filled best-effort so a diagnostic can name a
// crossing even when OK is clear.
func classifySignature(sig *types.Signature) FuncSig {
	ok := !sig.Variadic()
	params := make([]string, 0, sig.Params().Len())
	convs := make([]DefinedConv, 0, sig.Params().Len())
	paramElems := make([]string, 0, sig.Params().Len())
	for i := 0; i < sig.Params().Len(); i++ {
		t := sig.Params().At(i).Type()
		if elem, good := sliceCrossing(t); good {
			params = append(params, "slice")
			convs = append(convs, DefinedConv{})
			paramElems = append(paramElems, elem)
			continue
		}
		if path, name, good := opaqueCrossing(t); good {
			params = append(params, "opaque")
			convs = append(convs, DefinedConv{})
			paramElems = append(paramElems, OpaqueElem(path, name))
			continue
		}
		kw, conv, good := crossingType(t)
		if !good {
			ok = false
		}
		params = append(params, kw)
		convs = append(convs, conv)
		paramElems = append(paramElems, "")
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
		if elem, good := sliceCrossing(t); good {
			results = append(results, "slice")
			resultElems = append(resultElems, elem)
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
	return FuncSig{Params: params, ParamConv: convs, ParamElem: paramElems, Results: results, ResultElem: resultElems, ResultDefined: resultDefined, Throws: throws, OK: ok}
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

// sliceCrossing classifies a slice of a plain basic for the boundary, returning the
// element's Go type keyword and true so the lowerer marshals a []string or []float64
// element by element as a bento array (section 6.4). A []byte is deliberately not a
// slice crossing: it projects to a Uint8Array, a later slice (section 7.3), so it
// returns false and the call hands back. A slice of a defined type or a non-basic
// element also returns false, because only the scalar element crossings are covered
// here, so those await their own slice.
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
