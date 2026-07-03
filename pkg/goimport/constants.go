package goimport

import "go/types"

// This file exposes an exported Go constant's marshal keyword to the lowerer, the
// same way signature.go exposes a function's parameter and result types. A go:
// import can bind a constant (math.Pi, math.MaxInt32) and reference it as a value,
// and the lowerer marshals that reference by the constant's Go type, so a number
// constant crosses with the right conversion and range check (section 6.2, section
// 7.5). The TypeScript type alone projects every numeric constant as number, which
// does not say whether the Go value is an int, an int64, or a float64, so the
// crossing needs the real type the same way a call's does.

// Constants loads the Go package at importPath and returns the marshal keyword of
// each exported package-level constant whose type crosses the boundary, keyed by
// name. The keyword is the constant's Go type ("string", "bool", "int", "float64",
// and the rest of the numeric basics), which the lowerer reads to marshal a
// reference to the constant. An untyped constant (the common `const Pi = 3.14...`
// form) is classified by its default type, the type Go gives it when it is used in
// a context that needs one, because that is the type a reference to it takes. A
// constant of a defined type over a basic (time.Second over time.Duration) projects
// to a branded alias, not a plain number (section 6.11), so it is skipped here the
// same way classifySignature skips a defined-type parameter.
func Constants(importPath string) (map[string]ConstInfo, error) {
	pkg, err := loadPackage(importPath)
	if err != nil {
		return nil, err
	}
	return classifyConstants(pkg.Types), nil
}

// ConstInfo describes how a go: constant reference crosses the boundary. Keyword is
// the constant's underlying marshal keyword, the one the lowerer reads to marshal a
// reference. Defined records that the constant's type is a defined type over a basic
// (time.Second over time.Duration): the runtime value is the underlying number, but
// the read is of the branded type, so the lowerer strips the brand to the underlying
// basic before marshaling (section 6.11).
type ConstInfo struct {
	Keyword string
	Defined bool
}

// classifyConstants walks a package's exported package-level constants and returns
// how each one whose type crosses the boundary marshals, keyed by name. It is the
// shared classifier Constants runs over a loaded package and a test runs over a
// checked scope. An untyped constant is classified by its default type, a defined
// type over a basic crosses as its underlying value with the brand recorded, and a
// constant whose type has no clean crossing is dropped so the lowerer hands a
// reference to it back.
func classifyConstants(pkg *types.Package) map[string]ConstInfo {
	out := map[string]ConstInfo{}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		c, ok := obj.(*types.Const)
		if !ok {
			continue
		}
		// types.Default turns an untyped constant's type into the concrete default type a
		// reference takes (untyped float to float64, untyped int to int) and leaves a
		// typed constant's type unchanged. crossingType then classifies the plain basics
		// and the defined types over a basic, returning the underlying keyword and, for a
		// defined type, the conversion whose presence marks the brand.
		kw, conv, good := crossingType(types.Default(c.Type()))
		if !good {
			continue
		}
		out[name] = ConstInfo{Keyword: kw, Defined: conv.Name != ""}
	}
	return out
}
