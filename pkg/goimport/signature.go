package goimport

import "go/types"

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
type FuncSig struct {
	Params        []string
	ParamConv     []DefinedConv
	Results       []string
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
	for i := 0; i < sig.Params().Len(); i++ {
		kw, conv, good := crossingType(sig.Params().At(i).Type())
		if !good {
			ok = false
		}
		params = append(params, kw)
		convs = append(convs, conv)
	}
	var results []string
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
		kw, conv, good := crossingType(t)
		if !good {
			ok = false
		}
		if conv.Name != "" {
			resultDefined = true
		}
		results = append(results, kw)
	}
	if len(results) > 1 {
		// More than one non-error result maps to a tuple, a later slice; only a single
		// value (with or without a hoisted error) is marshaled today.
		ok = false
	}
	return FuncSig{Params: params, ParamConv: convs, Results: results, ResultDefined: resultDefined, Throws: throws, OK: ok}
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
