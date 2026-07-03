package goimport

import "go/types"

// This file exposes a go: package's exported sentinel error variables to the
// lowerer, the way constants.go exposes its constants. Go libraries branch on
// error identity against package-level error variables (io.EOF is the canonical
// one, sql.ErrNoRows another), and section 7.7 surfaces that branching to bento as
// err.is(EOF). For the lowerer to emit err.Is against the real Go variable it has
// to know that a bound name refers to an error variable rather than a function or
// a constant, which is what this classification reports.

// ErrorVars loads the Go package at importPath and returns the set of exported
// package-level variables whose type is an error, keyed by name. A bento author
// imports such a variable from a go: package and passes it to a caught error's
// is(), which lowers to errors.Is against the variable, so the runtime compares
// identity exactly as Go code does. Only variables are reported: an exported
// function or constant is not an error sentinel even if a value of it could be an
// error, so the lowerer keeps routing those through their own paths.
func ErrorVars(importPath string) (map[string]bool, error) {
	pkg, err := loadPackage(importPath)
	if err != nil {
		return nil, err
	}
	return classifyErrorVars(pkg.Types), nil
}

// classifyErrorVars walks a package's exported package-level variables and returns
// the set whose type is assignable to error, keyed by name. It is the shared
// classifier ErrorVars runs over a loaded package and a test runs over a checked
// scope. Assignability to the universe error interface is the test, so a variable
// typed error (io.EOF) and a variable of a concrete error type (a *MyError
// sentinel) both qualify, while a non-error variable is left out.
func classifyErrorVars(pkg *types.Package) map[string]bool {
	errorType := types.Universe.Lookup("error").Type()
	out := map[string]bool{}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		v, ok := obj.(*types.Var)
		if !ok {
			continue
		}
		if types.AssignableTo(v.Type(), errorType) {
			out[name] = true
		}
	}
	return out
}
