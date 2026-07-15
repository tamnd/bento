package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the typeof operator. typeof x is a string-valued expression
// whose result is one of a fixed set of tags ("number", "string", "function", and
// so on). When the checker already knows the operand's kind the whole thing folds
// to that tag as a string constant, so a typed program pays nothing for it; only a
// genuinely dynamic operand (any or unknown) defers the tag to runtime, where the
// boxed value reports its own kind through value.Value.TypeOf.

// isTypeofExpr reports whether n is a typeof expression. The shim does not expose a
// distinct kind for it, so typeof x surfaces as the catch-all NodeUnknown with the
// operand as its one child and the operator keyword leading its source text, the
// same shape-plus-text recognition the optional-chain token uses. A binding named
// something like typeofx never matches, because it lexes as an identifier node, not
// this catch-all, and because the leading keyword run would not equal "typeof".
func (r *Renderer) isTypeofExpr(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown || len(r.prog.Children(n)) != 1 {
		return false
	}
	return leadingKeyword(r.prog.Text(n)) == "typeof"
}

// typeofExpr lowers typeof x. A dynamic operand is evaluated once and asked for its
// runtime tag through value.Value.TypeOf, the one path where the kind is not known
// until the value exists. A statically typed operand folds to the tag as a string
// constant, which is sound only because the operand is proven side-effect free: the
// fold drops the operand from the output, and typeof otherwise evaluates it, so an
// operand that could run a call or an assignment hands back rather than lose that
// effect.
func (r *Renderer) typeofExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "typeof did not expose a single operand"}
	}
	operand := kids[0]

	// typeof over a caught error folds to the "object" tag: the runtime holds every
	// caught value as a *value.Error (catchDefer binds value.Caught), which is an
	// object, so the tag is known without lowering the binding, which has no general
	// value form. A thrown primitive that a catch recovers binds as an error too
	// (the ThrownString deviation), so it also answers "object" here, consistent
	// with how the model represents a caught value until the dynamic catch slice
	// binds the primitive itself. The binding is a plain identifier read with no
	// side effect, so dropping it from the output is sound.
	if r.isCaughtErrorRef(operand) {
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "FromGoString"),
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("object")}},
		}, nil
	}

	if r.isDynamic(operand) {
		e, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: e, Sel: ident("TypeOf")}}, nil
	}

	// typeof over a tagged-sum union of primitives spans more than one tag, so it
	// cannot fold to a constant, but the union value carries its tag, and each arm's
	// tag pins its typeof string. The operand is evaluated once and asked for that
	// string through the union's TypeOf method, so a side-effecting operand keeps its
	// effect rather than needing the repeatable-operand gate the fold takes. An object
	// or mixed union arm has no primitive typeof tag, so it stays on the handback.
	if info, ok := r.unionInfoOf(r.prog.TypeAt(operand)); ok {
		allPrim := true
		for _, a := range info.arms {
			if a.isObject {
				allPrim = false
				break
			}
		}
		if allPrim {
			e, err := r.lowerExpr(operand)
			if err != nil {
				return nil, err
			}
			info.needsTypeOf = true
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: e, Sel: ident("TypeOf")}}, nil
		}
	}

	tag, ok := r.staticTypeofTag(operand)
	if !ok {
		return nil, &NotYetLowerable{Reason: "typeof over this static type is a later slice"}
	}
	if !r.repeatableOperand(operand) {
		return nil, &NotYetLowerable{Reason: "typeof over an operand with a side effect is a later slice"}
	}
	return &ast.CallExpr{
		Fun:  sel("value", "FromGoString"),
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(tag)}},
	}, nil
}

// staticTypeofTag returns the typeof tag the checker's type for n pins down, and
// ok=false when the type does not determine one tag. The mapping is JavaScript's:
// null answers "object" (the historical wart), a plain object and an array both
// answer "object", and only a callable answers "function", distinguished by the
// call signatures its type carries. A union (including an optional T | undefined)
// spans more than one tag and so returns false, deferring that operand to a later
// slice rather than guessing.
func (r *Renderer) staticTypeofTag(n frontend.Node) (string, bool) {
	f := r.primitiveFlags(n)
	switch {
	case f&frontend.TypeNumber != 0:
		return "number", true
	case f&frontend.TypeString != 0:
		return "string", true
	case f&frontend.TypeBoolean != 0:
		return "boolean", true
	case f&frontend.TypeBigInt != 0:
		return "bigint", true
	case f&frontend.TypeSymbol != 0:
		return "symbol", true
	case f&(frontend.TypeUndefined|frontend.TypeVoid) != 0:
		return "undefined", true
	case f&frontend.TypeNull != 0:
		return "object", true
	}
	if f&frontend.TypeObject != 0 {
		// A callable answers "function", and so does a class: its type carries
		// construct signatures rather than call signatures, but typeof a class
		// constructor is "function" in JavaScript all the same. Checking only
		// call signatures folded typeof SomeClass to "object", which fails the
		// harness sta.js self-test (typeof Test262Error === "function").
		if call, construct := r.prog.Signatures(r.prog.TypeAt(n)); len(call) > 0 || len(construct) > 0 {
			return "function", true
		}
		return "object", true
	}
	return "", false
}

// leadingKeyword returns the run of ASCII letters at the start of s, the operator
// keyword a prefix expression like typeof x or void x leads with. It stops at the
// first non-letter (the space before the operand), so the whole keyword is returned
// and nothing of the operand, and a name that merely starts with the letters comes
// back longer than the keyword and so fails an equality test against it.
func leadingKeyword(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			break
		}
		i++
	}
	return s[:i]
}
