package lower

import (
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A generic function has no single Go form: `identity<T>(x: T): T` names a
// different Go signature for every T a call site fixes it to. Bento answers that
// by monomorphization, emitting one specialized Go function per distinct
// instantiation the program asks for, the same shape the typed-tier design
// (2075/typed/08_generics_and_functions.md) names T10. The call `identity(5)`
// resolves against a specialization named Identity_num typed `func(float64)
// float64`; `identity("hi")` against Identity_str typed `func(value.BStr)
// value.BStr`. This file runs the discovery pass that finds those instantiations
// before any body lowers, mangles a specialization name from the concrete type
// arguments, and gives the call site the name it resolves to. The body of each
// specialization lowers with typeSubst active, so typeExpr resolves the bare T to
// the concrete type this call fixed (see lower.go typeExpr).
//
// Soundness is the same handback contract the rest of lowering keeps: a generic
// is specialized only when every type argument a call fixes it to mangles to a
// concrete Go type. A type argument that is dynamic, or a shape the atom table
// does not name, leaves the generic unspecialized, and both the declaration and
// its call sites hand back rather than emit an unsound Go form.

// monoSpec is one monomorphization of a generic function: the concrete type each
// of its type parameters was fixed to (subst, keyed by type-parameter identity)
// and the suffix those types mangle to, which names the specialized Go function
// (Identity_num, First_arr_str). Two call sites that fix the same Go types share
// one spec, since the suffix is derived from the lowered types and dedups them.
type monoSpec struct {
	suffix string
	subst  map[int]frontend.Type
}

// collectMono walks the whole program before any body lowers and records, per
// generic top-level function, the distinct monomorphizations its call sites ask
// for. It mirrors the other RenderProgram pre-passes (collectClasses,
// collectEnums): a whole-program walk that fills a Renderer map the declaration
// and call-site lowering both read, so a call below a generic declaration and the
// declaration itself agree on which specializations exist without a shared table.
func (r *Renderer) collectMono(entry frontend.Node) {
	seen := map[frontend.Symbol]map[string]bool{}
	var walk func(frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeCallExpression {
			if sym, spec, ok := r.monoCallSpec(n); ok {
				bySuffix := seen[sym]
				if bySuffix == nil {
					bySuffix = map[string]bool{}
					seen[sym] = bySuffix
				}
				if !bySuffix[spec.suffix] {
					bySuffix[spec.suffix] = true
					r.monoSpecs[sym] = append(r.monoSpecs[sym], spec)
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
}

// monoCallSpec reports the monomorphization a call resolves to when its callee is
// a generic top-level function every type argument of which mangles to a concrete
// Go type. It returns the callee symbol and the spec, or ok=false when the callee
// is not such a generic (a plain function, a method, a value binding) or when a
// type argument does not mangle, in which case the generic is left unspecialized
// and hands back at both the declaration and the call. It is the one place the
// call-to-specialization mapping is computed, so discovery and the call-site
// rewrite (monoCalleeName) read the same answer.
func (r *Renderer) monoCallSpec(call frontend.Node) (frontend.Symbol, monoSpec, bool) {
	kids := r.prog.Children(call)
	if len(kids) == 0 {
		return frontend.Symbol{}, monoSpec{}, false
	}
	callee := kids[0]
	if callee.Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, monoSpec{}, false
	}
	sym, ok := r.prog.SymbolAt(callee)
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return frontend.Symbol{}, monoSpec{}, false
	}
	decl, ok := r.genericFuncDecl(sym)
	if !ok {
		return frontend.Symbol{}, monoSpec{}, false
	}
	declSig, ok := r.prog.SignatureAt(decl)
	if !ok || len(declSig.TypeParams) == 0 {
		return frontend.Symbol{}, monoSpec{}, false
	}
	resSig, ok := r.prog.SignatureAt(call)
	if !ok {
		return frontend.Symbol{}, monoSpec{}, false
	}
	subst := map[int]frontend.Type{}
	if !r.unifyGeneric(declSig, resSig, subst) {
		return frontend.Symbol{}, monoSpec{}, false
	}
	suffix, ok := r.monoSuffix(declSig.TypeParams, subst)
	if !ok {
		return frontend.Symbol{}, monoSpec{}, false
	}
	return sym, monoSpec{suffix: suffix, subst: subst}, true
}

// genericFuncDecl returns the function-declaration node of a generic top-level
// function the symbol names, or ok=false when the symbol is not a single
// function declaration (an overload set, a merged binding) or is not generic.
// Only a plain function declaration is monomorphized here; a generic method or a
// generic function value is its own later slice.
func (r *Renderer) genericFuncDecl(sym frontend.Symbol) (frontend.Node, bool) {
	decls := r.prog.Declarations(sym)
	if len(decls) != 1 {
		return nil, false
	}
	decl := decls[0]
	if decl.Kind() != frontend.NodeFunctionDeclaration {
		return nil, false
	}
	sig, ok := r.prog.SignatureAt(decl)
	if !ok || len(sig.TypeParams) == 0 {
		return nil, false
	}
	return decl, true
}

// unifyGeneric binds each type parameter of a generic declaration to the concrete
// type the resolved call signature fixed it to, by walking the declaration's
// parameter and return types against the resolved ones in parallel. Where the
// declaration type is a bare type parameter the concrete type across from it is
// its binding; an array of a type parameter binds through its element. It reports
// false when a binding conflicts (the same parameter fixed to two different types,
// which a well-typed call never produces) or when a type parameter is left unbound
// because it appears only in a form this pass does not walk, in which case the
// generic is not specialized and hands back.
func (r *Renderer) unifyGeneric(decl, res frontend.Signature, subst map[int]frontend.Type) bool {
	for i, p := range decl.Params {
		if i >= len(res.Params) {
			return false
		}
		if !r.unifyType(p.Type, res.Params[i].Type, subst) {
			return false
		}
	}
	if !r.unifyType(decl.Return, res.Return, subst) {
		return false
	}
	for _, tp := range decl.TypeParams {
		if _, ok := subst[tp.Identity()]; !ok {
			return false
		}
	}
	return true
}

// unifyType matches one declaration type against the concrete type across from it,
// recording a binding wherever the declaration type is a bare type parameter. It
// descends through an array so `T[]` fixed against `number[]` binds T to number. A
// declaration type that carries no type parameter contributes no binding and is
// satisfied trivially, since its Go form does not depend on the instantiation. A
// conflicting rebind of one parameter reports false so the caller hands back.
func (r *Renderer) unifyType(decl, conc frontend.Type, subst map[int]frontend.Type) bool {
	if decl.Flags&frontend.TypeTypeParameter != 0 {
		// A literal argument fixes a type parameter to its literal type (firstOf(10, 20)
		// binds T to the literals 10 and 20, not number), so the binding widens to the
		// type the specialized Go slot takes, the same widening a literal undergoes when
		// it escapes its context. A type parameter named twice then agrees whenever the
		// two arguments widen to one Go type, the dedup-at-the-lowered-type rule the
		// suffix uses, so firstOf(10, 20) and firstOf(true, false) each bind cleanly.
		conc = r.prog.Widen(conc)
		if prev, ok := subst[decl.Identity()]; ok {
			pa, pok := r.monoAtom(prev)
			ca, cok := r.monoAtom(conc)
			return pok && cok && pa == ca
		}
		subst[decl.Identity()] = conc
		return true
	}
	if de, ok := r.prog.ElementType(decl); ok {
		ce, ok := r.prog.ElementType(conc)
		if !ok {
			return true
		}
		return r.unifyType(de, ce, subst)
	}
	return true
}

// monoSuffix mangles the concrete type arguments of one instantiation into the
// suffix its specialized Go name carries, ordered by the declaration's type
// parameters so two call sites that fix the same types produce the same suffix. It
// reports false when any type argument does not name a concrete Go type this slice
// mangles, which leaves the generic unspecialized. The suffix is derived from the
// lowered Go type, so a branded alias and its underlying primitive collapse to one
// specialization the same way their Go types do.
func (r *Renderer) monoSuffix(typeParams []frontend.Type, subst map[int]frontend.Type) (string, bool) {
	var atoms []string
	for _, tp := range typeParams {
		conc, ok := subst[tp.Identity()]
		if !ok {
			return "", false
		}
		atom, ok := r.monoAtom(conc)
		if !ok {
			return "", false
		}
		atoms = append(atoms, atom)
	}
	if len(atoms) == 0 {
		return "", false
	}
	return strings.Join(atoms, "_"), true
}

// monoAtom names the concrete Go type a type argument lowers to with a short atom
// the specialized name reads by (num, str, bool, big, arr_num). It reports false
// for a type argument this slice does not monomorphize: a dynamic type argument,
// which routes to the boxed fallback of a later slice, and any object shape other
// than an array of a mangleable element, whose stable naming is a later slice. The
// atom is injective with respect to the lowered Go type within this table, so two
// distinct Go types never share a suffix and collapse into one specialization.
func (r *Renderer) monoAtom(t frontend.Type) (string, bool) {
	// A dynamic type argument routes to the boxed fallback of a later slice.
	if t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return "", false
	}
	// A type argument inferred as a union of literals (firstOf(10, 20) fixes T to
	// 10 | 20, firstOf(true, false) to true | false) lowers by the same fold
	// typeExpr runs: a union all of whose members carry the number or boolean facet
	// widens to that primitive's Go type. primitiveFlagsOfType folds the facet in
	// whether or not the union already spells it, so both spellings mangle here.
	// Only number and boolean fold; a string-literal union lowers to a tag enum,
	// not value.BStr, and mixed or nullable unions have their own Go shape, so both
	// are left to a later slice rather than mangled.
	if t.Flags&frontend.TypeUnion != 0 && t.Flags&(frontend.TypeNumber|frontend.TypeBoolean|frontend.TypeString) == 0 {
		switch pf := r.primitiveFlagsOfType(t); {
		case pf&frontend.TypeNumber != 0:
			return "num", true
		case pf&frontend.TypeBoolean != 0:
			return "bool", true
		default:
			return "", false
		}
	}
	switch {
	case t.Flags&frontend.TypeNumber != 0:
		return "num", true
	case t.Flags&frontend.TypeString != 0:
		// A closed string-literal union lowers to an integer tag enum, not value.BStr,
		// so only a type argument that is string (or a lone string literal, which widens
		// to string) mangles here; a genuine string-literal union is caught above.
		if t.Flags&frontend.TypeUnion != 0 {
			return "", false
		}
		return "str", true
	case t.Flags&frontend.TypeBoolean != 0:
		return "bool", true
	case t.Flags&frontend.TypeBigInt != 0:
		return "big", true
	case t.Flags&frontend.TypeObject != 0:
		if elem, ok := r.prog.ElementType(t); ok {
			atom, ok := r.monoAtom(elem)
			if !ok {
				return "", false
			}
			return "arr_" + atom, true
		}
		return "", false
	default:
		return "", false
	}
}

// monoCalleeName returns the specialized Go name a call to a generic function
// resolves to, or ok=false when the callee is not a monomorphized generic so the
// caller keeps the plain exported name. It reports an error only when the callee is
// a generic bento discovered as specializable but this call's type arguments do
// not mangle, which cannot happen once collectMono has run but is guarded so a
// mismatch hands back rather than name a Go function that was never emitted.
func (r *Renderer) monoCalleeName(call frontend.Node) (string, bool, error) {
	kids := r.prog.Children(call)
	if len(kids) == 0 {
		return "", false, nil
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok {
		return "", false, nil
	}
	if len(r.monoSpecs[sym]) == 0 {
		return "", false, nil
	}
	base, ok := exportedField(sym.Name)
	if !ok {
		return "", false, nil
	}
	_, spec, ok := r.monoCallSpec(call)
	if !ok {
		return "", false, &NotYetLowerable{Reason: "a generic call whose type arguments do not monomorphize is a later slice"}
	}
	return base + "_" + spec.suffix, true, nil
}
