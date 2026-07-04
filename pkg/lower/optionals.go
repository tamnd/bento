package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file handles optional locals: the analysis that finds locals holding a
// T | undefined the pointer form models, and the undefined-comparison shapes
// that read them.

// boxToOptional wraps a value flowing into an optional slot, value.Opt[T], in the
// Some or None constructor the slot's element type spells. It is the boxing side
// of the optional lowering: reads unwrap with Get and presence tests use
// IsUndefined, and this is where a bare T or a bare undefined becomes the Opt the
// slot holds. A source already of the optional shape is the same Opt and passes
// straight through, so an optional argument bound to an optional parameter is not
// double-wrapped. It returns (expr, false, nil) when the target is not an optional
// or the source is already one, leaving the caller on its existing path.
func (r *Renderer) boxToOptional(expr ast.Expr, src frontend.Node, target frontend.Type) (ast.Expr, bool, error) {
	if !r.isOptionalType(target) || r.isOptional(src) {
		return expr, false, nil
	}
	inner, ok := r.optionalInner(r.prog.UnionMembers(target))
	if !ok {
		return expr, false, nil
	}
	elem, err := r.typeExpr(inner)
	if err != nil {
		return nil, false, err
	}
	r.requireImport(valuePkg)
	// A source typed exactly undefined is the empty optional; None takes no value,
	// so the placeholder lowering of the undefined identifier is dropped here.
	if r.prog.TypeAt(src).Flags == frontend.TypeUndefined {
		return &ast.CallExpr{Fun: index(sel("value", "None"), elem)}, true, nil
	}
	// A dynamic source has no static T to store in the slot; boxing it needs the
	// dynamic-to-static coercion a later slice adds, so it hands back.
	if r.isDynamic(src) {
		return nil, false, &NotYetLowerable{Reason: "boxing a dynamic value into an optional slot is a later slice"}
	}
	// The source is a present value. Bridge it to the element type first so a
	// derived instance upcasts to the base the optional declares, then wrap it.
	bridged, err := r.bridgeClassBinding(expr, src, inner)
	if err != nil {
		return nil, false, err
	}
	return &ast.CallExpr{Fun: index(sel("value", "Some"), elem), Args: []ast.Expr{bridged}}, true, nil
}

// optionalUndefinedCompare recognizes an equality between an optional and the
// bare undefined literal and returns the optional operand. One operand must type
// as exactly undefined (the undefined keyword, flags TypeUndefined) and the other
// must be an optional (a union whose members are the T | undefined shape). It
// returns false when neither operand is the undefined literal, when both are, or
// when the non-undefined operand is not an optional, so the caller only rewrites
// the genuine presence test and leaves every other equality to the value compare.
func (r *Renderer) optionalUndefinedCompare(left, right frontend.Node) (frontend.Node, bool) {
	lUndef := r.prog.TypeAt(left).Flags == frontend.TypeUndefined
	rUndef := r.prog.TypeAt(right).Flags == frontend.TypeUndefined
	switch {
	case rUndef && !lUndef && r.isOptional(left):
		return left, true
	case lUndef && !rUndef && r.isOptional(right):
		return right, true
	default:
		return nil, false
	}
}

// isOptional reports whether a node's type is an optional, the T | undefined
// shape that lowers to value.Opt[T]. It reads the type as a union and checks the
// optional shape the same way renderUnion does, so the presence-test rewrite
// fires exactly when the operand is a value.Opt and not on a wider union.
func (r *Renderer) isOptional(n frontend.Node) bool {
	return r.isOptionalType(r.prog.TypeAt(n))
}

// isOptionalType reports whether a type is the optional T | undefined shape that
// lowers to value.Opt[T], reading it the same way renderUnion does. It is the
// node-free form isOptional and the optLocals pre-pass share, so the declaration
// scan and the per-use narrowing test agree on what counts as an option.
func (r *Renderer) isOptionalType(t frontend.Type) bool {
	if t.Flags&frontend.TypeUnion == 0 {
		return false
	}
	_, ok := r.optionalInner(r.prog.UnionMembers(t))
	return ok
}

// optLocalsOf analyzes a body and returns the set of local names declared with an
// optional type (T | undefined, lowered to value.Opt[T]), so a read of one at a
// point the checker narrowed to T can unwrap with .Get(). The walk descends through
// nested blocks like int32LocalsOf, and it reads the declared type from the name
// node of each variable declaration, which is the unnarrowed type at the point of
// declaration. A name declared more than once is dropped from the set, since the
// flat name set cannot tell two scopes apart and a wrong unwrap would be unsound;
// such a body simply keeps every read of that name bare and hands back the narrowed
// use to a later slice rather than risk it. A nil map means nothing to unwrap.
func (r *Renderer) optLocalsOf(body []frontend.Node) map[string]bool {
	opt := map[string]bool{}
	declCount := map[string]int{}
	for _, n := range body {
		r.collectOptDecls(n, opt, declCount)
	}
	for name, c := range declCount {
		if c != 1 {
			delete(opt, name)
		}
	}
	if len(opt) == 0 {
		return nil
	}
	return opt
}

// collectOptDecls walks one node, recording each variable declaration whose name is
// typed as an optional, and recurses into its children so a binding in a nested
// block or loop is seen. It counts declarations per name alongside so optLocalsOf
// can drop a name declared in more than one scope.
func (r *Renderer) collectOptDecls(n frontend.Node, opt map[string]bool, declCount map[string]int) {
	if n.Kind() == frontend.NodeVariableDeclaration {
		kids := r.prog.Children(n)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			if name, ok := localName(r.prog.Text(kids[0])); ok {
				declCount[name]++
				if r.isOptionalType(r.prog.TypeAt(kids[0])) {
					opt[name] = true
				}
			}
		}
	}
	for _, c := range r.prog.Children(n) {
		r.collectOptDecls(c, opt, declCount)
	}
}
