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
	// A source already of the optional shape is the same Opt and passes through, but
	// only when it is a static optional: a dynamic source the checker types
	// T | undefined is a boxed value.Value, not an Opt[T], so it still needs the
	// unboxing coercion below rather than passing straight into the slot.
	if !r.isOptionalType(target) || (r.isOptional(src) && !r.isDynamic(src)) {
		return expr, false, nil
	}
	inner, ok := r.optionalInner(r.prog.UnionMembers(target))
	if !ok {
		return expr, false, nil
	}
	// An optional whose inner lowers to the dynamic value.Value box does not become an
	// Opt[value.Value] slot: the box already carries undefined, so renderUnion collapses
	// { } | undefined to a bare value.Value. Wrapping such a value in Some or None would
	// spell an Opt the Go slot never has, so it declines here and the plain dynamic bridge
	// boxes or passes the value the way any other box move does.
	if inner.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 || r.isNarrowableBoxType(inner) {
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
	// A dynamic source, a boxed value.Value, unboxes into the optional through the
	// value model's dynamic-to-optional coercers: undefined lands as the empty optional
	// and any other value coerces to the element primitive, the same ToNumber the
	// non-optional dynamic coercion runs, wrapped so a miss stays undefined. A member
	// read off a { } value narrowed by a user type guard returning into a
	// number | undefined slot is the motivating case. A non-primitive element has no
	// such coercer yet and hands back.
	if r.isDynamic(src) {
		r.requireImport(valuePkg)
		switch {
		case inner.Flags&frontend.TypeNumber != 0:
			return &ast.CallExpr{Fun: sel("value", "ToOptNumber"), Args: []ast.Expr{expr}}, true, nil
		case inner.Flags&frontend.TypeString != 0:
			return &ast.CallExpr{Fun: sel("value", "ToOptString"), Args: []ast.Expr{expr}}, true, nil
		case inner.Flags&frontend.TypeBoolean != 0:
			return &ast.CallExpr{Fun: sel("value", "ToOptBoolean"), Args: []ast.Expr{expr}}, true, nil
		}
		return nil, false, &NotYetLowerable{Reason: "boxing a dynamic value into a non-primitive optional slot is a later slice"}
	}
	// The source is a present value. Bridge it to the element type first so a
	// derived instance upcasts to the base the optional declares, then wrap it.
	bridged, err := r.bridgeClassBinding(expr, src, inner)
	if err != nil {
		return nil, false, err
	}
	return &ast.CallExpr{Fun: index(sel("value", "Some"), elem), Args: []ast.Expr{bridged}}, true, nil
}

// isPlainShape reports whether a type is a fixed-shape object that lowers to an
// interned struct: an object that is not an array, not a class instance (which
// has its own generated type and upcast bridge), and not a union. It is the
// shape test the contextual object-literal build and the shape-cross guard
// share, so both agree on which types the struct interner owns.
func (r *Renderer) isPlainShape(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 || t.Flags&frontend.TypeUnion != 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	if _, isClass := r.classOfType(t); isClass {
		return false
	}
	return true
}

// shapeHasOptional reports whether a fixed shape declares any optional property,
// the shapes whose interning this slice added and whose crossings the guard
// below polices.
func (r *Renderer) shapeHasOptional(t frontend.Type) bool {
	for _, p := range r.prog.Properties(t) {
		if p.Optional {
			return true
		}
	}
	return false
}

// shapeProp returns the declared property of a fixed shape by its source name,
// the declaration a member read or write consults for the property's unnarrowed
// type and optionality.
func (r *Renderer) shapeProp(t frontend.Type, name string) (frontend.Property, bool) {
	for _, p := range r.prog.Properties(t) {
		if p.Name == name {
			return p, true
		}
	}
	return frontend.Property{}, false
}

// contextualObjectShape returns the fixed shape an object literal in a slot of
// the declared type must build at, unwrapping a declared T | undefined to its
// inner shape (wrap reports that unwrap so the caller re-wraps the built value
// in Some). It reports ok only when the shape declares an optional property:
// that is the one case where the literal's own fresh type, whose members are
// all required, interns a different struct than the slot declares, so the
// literal must build at the declared shape, the same contextual typing
// TypeScript applies to it. Every other literal keeps building at its own
// type, unchanged behavior.
func (r *Renderer) contextualObjectShape(declared frontend.Type) (shape frontend.Type, wrap, ok bool) {
	shape = declared
	if inner, isOpt := r.optionalInner(r.prog.UnionMembers(declared)); isOpt {
		shape, wrap = inner, true
	}
	if !r.isPlainShape(shape) || !r.shapeHasOptional(shape) {
		return frontend.Type{}, false, false
	}
	return shape, wrap, true
}

// someWrap wraps a present value in value.Some at the given element type, the
// boxing a contextual literal applies to a present optional field.
func (r *Renderer) someWrap(expr ast.Expr, elem frontend.Type) (ast.Expr, error) {
	elemGo, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "Some"), elemGo), Args: []ast.Expr{expr}}, nil
}

// noneOf is the empty optional value.None[T]() at the given element type, the
// value a contextual literal supplies for an optional field its members omit.
func (r *Renderer) noneOf(elem frontend.Type) (ast.Expr, error) {
	elemGo, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "None"), elemGo)}, nil
}

// guardOptionalShapeCross hands back when a value flows into a slot of a
// different fixed shape and either shape carries an optional property. Two
// different shapes intern to two different Go structs, so such a move never
// compiles even though TypeScript's width subtyping accepts it; before optional
// properties interned, these targets handed back at internStruct, and this
// guard keeps that routing for the sources the contextual literal build does
// not cover (a variable of a fresh required shape passed where the optional
// shape is declared). Two crossings of plain shapes keep their pre-optional
// behavior and are not this guard's slice. An optional target slot guards
// against its inner shape, since that is the type the wrapped value must have.
func (r *Renderer) guardOptionalShapeCross(src frontend.Node, target frontend.Type) error {
	if inner, ok := r.optionalInner(r.prog.UnionMembers(target)); ok {
		target = inner
	}
	return r.guardOptionalShapeCrossTypes(r.prog.TypeAt(src), target)
}

// guardOptionalShapeCrossTypes is the type-level core of guardOptionalShapeCross,
// for the spread copy that reads fields off a source type with no per-field node.
func (r *Renderer) guardOptionalShapeCrossTypes(src, target frontend.Type) error {
	if !r.isPlainShape(src) || !r.isPlainShape(target) {
		return nil
	}
	if structuralKey(r.prog, src, map[int]int{}) == structuralKey(r.prog, target, map[int]int{}) {
		return nil
	}
	if r.shapeHasOptional(src) || r.shapeHasOptional(target) {
		return &NotYetLowerable{Reason: "an object crossing to a slot of a different shape with an optional property is a later slice"}
	}
	return nil
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

// optionalSlotUndefinedCompare recognizes an equality between the bare undefined
// literal and an identifier that binds an optional slot (value.Opt[T]) whose type
// the checker has narrowed away from the optional union at this use, and returns
// the name. It is the shape a write leaves behind: after `n = 42` the checker knows
// n holds a number, so the operand no longer types as optional and
// optionalUndefinedCompare declines it, but the Go slot stays Opt[T] and holds Some,
// so the presence test is a valid n.IsUndefined() that answers false, exactly what
// `42 === undefined` evaluates to. The name is returned rather than the node so the
// caller reads the raw slot: lowerExpr of a narrowed optBinding emits the .Get() a
// value read wants, which would not carry IsUndefined.
func (r *Renderer) optionalSlotUndefinedCompare(left, right frontend.Node) (string, bool) {
	if name, ok := r.optSlotName(left); ok && r.prog.TypeAt(right).Flags == frontend.TypeUndefined {
		return name, true
	}
	if name, ok := r.optSlotName(right); ok && r.prog.TypeAt(left).Flags == frontend.TypeUndefined {
		return name, true
	}
	return "", false
}

// optSlotName returns the name of an identifier that binds an optional slot,
// whether a local declared T | undefined or an optional parameter, so the presence
// test can read the raw value.Opt[T] regardless of how the checker has narrowed the
// reference at this use.
func (r *Renderer) optSlotName(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodeIdentifier {
		return "", false
	}
	name, ok := localName(r.prog.Text(n))
	if !ok || !r.isOptBinding(name) {
		return "", false
	}
	return name, true
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

// isOptBinding reports whether name binds an optional (value.Opt[T]) in the body
// being lowered, whether it is a local declared with an optional type or a parameter
// whose field is an option, the bare x?: T form or a required x: T | undefined. The
// two sets are kept apart because a local's optional-ness is
// recomputed per block from the body's declarations while a parameter's rides the
// signature, but a narrowed read unwraps either the same way, so the read sites
// consult this one predicate.
func (r *Renderer) isOptBinding(name string) bool {
	return r.optLocals[name] || r.optParams[name]
}

// optParamsOf returns the set of parameter names whose field is a value.Opt[T] the
// body reads through .Get() at a narrowed use. Two shapes qualify. A bare optional,
// the x?: T form at or past MinArgs with no default, whose field funcParamFields
// lowers to Opt[T]. And a required parameter annotated x: T | undefined, before
// MinArgs, whose field paramFieldType already renders to Opt[T] through typeExpr
// because the type is the two-member optional union; the caller always supplies it,
// as Some for a present value or None for an explicit undefined, so no call-site
// defaulting is involved, but a read the checker narrowed to T still has to unwrap.
// A defaulted optional binds the plain T the default fills, not an option, so it is
// excluded, as is a dynamic optional (any or unknown), which binds a boxed value that
// holds undefined natively. It is built once per body so a narrowed read of the
// parameter unwraps with .Get() wherever in the body it sits.
func (r *Renderer) optParamsOf(fn frontend.Node, sig frontend.Signature) map[string]bool {
	paramNodes := r.funcParamNodes(fn)
	var opt map[string]bool
	for i, p := range sig.Params {
		if !r.isOptionalType(p.Type) {
			continue
		}
		// An optional whose inner lowers to the dynamic value.Value box, { } | undefined
		// the motivating case, binds a bare value.Value the box holds undefined in
		// natively (renderUnion collapses it), not a value.Opt[T]. A narrowed read of it
		// dispatches through the box, not an Opt unwrap, so it is excluded here the same as
		// a dynamic any or unknown optional is.
		if r.isNarrowableBoxType(p.Type) {
			continue
		}
		// A defaulted optional binds the plain T the default fills, so its field is
		// not an option; only an omittable parameter can carry a default, so this is
		// checked past MinArgs alone.
		if i >= sig.MinArgs {
			if _, hasDef := r.paramDefaultNode(paramNodes, i); hasDef {
				continue
			}
		}
		name, ok := localName(p.Name)
		if !ok {
			continue
		}
		if opt == nil {
			opt = map[string]bool{}
		}
		opt[name] = true
	}
	return opt
}

// pushOptParams sets the body-scoped optional-parameter narrowing set and returns a
// restore func the caller defers, so a method, async, or generator body lowers a narrowed
// read of a tracked parameter through .Get() the way a top-level function body does. The
// set survives the body lowering because scopedBlockRange recomputes optLocals per block
// but leaves optParams alone.
func (r *Renderer) pushOptParams(set map[string]bool) func() {
	prev := r.optParams
	r.optParams = set
	return func() { r.optParams = prev }
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
