package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A module-level destructuring binding is a module binding like any other. `const [p, q]
// = arr` declares p and q at module scope, so a top-level function that reads either one
// reads module state, and the binding has to reach package scope the way a plainly named
// one does.
//
// The hoist never saw them. It read a declaration's name off its first child and asked
// localName for a Go identifier, and a pattern's first child is the pattern itself, whose
// text is `[p, q]`. No name matched the cross-boundary set, so the statement stayed a
// local of main and the function that named p referred to nothing: the emitted Go said
// `undefined: p`. That is a program that does not build rather than a hand-back, and it
// reproduces with a fully static `number[]` source, so it never had anything to do with
// what the pattern binds.
//
// This file is the pattern side of the three questions the plain path already answers:
// which names a declaration introduces, what each one's package var is, and how the
// statement that stays in main stores into them.

// patternLeafNodes collects the identifier nodes a binding pattern introduces, in source
// order, following a nested pattern down to its own leaves. It reads each element through
// the same classifiers the destructuring lowering reads it through, so the names it
// reports and the names that lowering binds are the same list by construction.
//
// It reports false for any element shape those classifiers decline, a hole among other
// things. The caller treats that as unhoistable and hands back, which is where such a
// pattern already stops when it lowers.
func (r *Renderer) patternLeafNodes(pat frontend.Node, out *[]frontend.Node) bool {
	txt := strings.TrimSpace(r.prog.Text(pat))
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return false
	}
	switch {
	case strings.HasPrefix(txt, "["):
		fixed, restNode, hasRest, err := r.splitArrayRest(elems)
		if err != nil {
			return false
		}
		for _, el := range fixed {
			info, err := r.classifyArrayElem(el)
			if err != nil {
				return false
			}
			if info.nested != nil {
				if !r.patternLeafNodes(info.nested, out) {
					return false
				}
				continue
			}
			*out = append(*out, info.nameNode)
		}
		if hasRest {
			*out = append(*out, restNode)
		}
		return true
	case strings.HasPrefix(txt, "{"):
		for _, el := range elems {
			if node, ok := r.objectRestElem(el); ok {
				*out = append(*out, node)
				continue
			}
			if _, sub, ok := r.objectNestedElem(el); ok {
				if !r.patternLeafNodes(sub, out) {
					return false
				}
				continue
			}
			info, err := r.classifyObjectElem(el)
			if err != nil {
				return false
			}
			*out = append(*out, info.bindNode)
		}
		return true
	}
	return false
}

// declBindingNames returns the identifier nodes one module-level variable declaration
// introduces: the name itself for a plain binding, every leaf for a pattern. It is what
// the hoist asks instead of reading the first child and hoping it is a name.
func (r *Renderer) declBindingNames(d frontend.Node) ([]frontend.Node, bool) {
	kids := r.prog.Children(d)
	if len(kids) == 0 {
		return nil, false
	}
	if kids[0].Kind() == frontend.NodeIdentifier {
		return []frontend.Node{kids[0]}, true
	}
	if !r.patternNode(kids[0]) {
		return nil, false
	}
	var out []frontend.Node
	if !r.patternLeafNodes(kids[0], &out) {
		return nil, false
	}
	return out, true
}

// declIsPattern reports whether a variable declaration binds a destructuring pattern
// rather than a plain name.
func (r *Renderer) declIsPattern(d frontend.Node) bool {
	kids := r.prog.Children(d)
	return len(kids) > 0 && r.patternNode(kids[0])
}

// modulePatternZeroSpecs builds the zero-valued package vars for a hoisted destructuring
// declaration, one per name the pattern introduces. The statement stays in main and its
// binds assign into these, so each spec carries the type the leaf's own bind produces.
//
// A leaf whose slot holds a box declares the package var as one, since the bind that
// stays in main stores a value.Value into it and every read of the name dispatches
// through the value model wherever it is.
//
// One shape hands back rather than declare a var the bind cannot fill: a leaf whose
// declared type and narrowed type render to different Go types, since the bind picks
// between them per element shape and a package var can only spell one.
func (r *Renderer) modulePatternZeroSpecs(d frontend.Node) ([]ast.Spec, error) {
	names, ok := r.declBindingNames(d)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a module destructuring pattern a function reads has an element shape the hoist cannot name"}
	}
	kids := r.prog.Children(d)
	initNode := kids[len(kids)-1]
	specs := make([]ast.Spec, 0, len(names))
	for _, nn := range names {
		name, ok := localName(r.prog.Text(nn))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a hoisted module binding name is not a Go identifier"}
		}
		if r.patternLeafBindsABox(nn, initNode) {
			r.requireImport(valuePkg)
			specs = append(specs, &ast.ValueSpec{
				Names: []*ast.Ident{ident(name)},
				Type:  sel("value", "Value"),
			})
			continue
		}
		declGo, err := r.typeExpr(r.bindingDeclaredType(nn))
		if err != nil {
			return nil, err
		}
		narrowGo, err := r.typeExpr(r.prog.TypeAt(nn))
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(declGo, narrowGo); err != nil {
			return nil, err
		} else if !same {
			return nil, &NotYetLowerable{Reason: "a module destructuring binding whose declared and narrowed types render differently is a later slice"}
		}
		specs = append(specs, &ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: declGo})
	}
	return specs, nil
}

// markModulePatternAssign records that a hoisted destructuring declaration's leaves are
// package vars its in-main statement assigns into, keyed by symbol so a same-named binding
// somewhere else in the program is untouched.
func (r *Renderer) markModulePatternAssign(d frontend.Node) {
	names, ok := r.declBindingNames(d)
	if !ok {
		return
	}
	for _, nn := range names {
		if sym, ok := r.prog.SymbolAt(nn); ok {
			r.moduleAssignPatternNames[sym] = true
		}
		if name, ok := localName(r.prog.Text(nn)); ok {
			r.moduleAssignVars[name] = true
		}
	}
}

// hoistedPatternTargets returns the Go names a destructuring statement binds that are
// package vars rather than fresh main locals. A statement with none returns nil and lowers
// untouched, which is every destructuring inside a function body and every module-level
// one no function reads.
func (r *Renderer) hoistedPatternTargets(n frontend.Node) map[string]bool {
	if len(r.moduleAssignPatternNames) == 0 || n.Kind() != frontend.NodeVariableStatement {
		return nil
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	out := map[string]bool{}
	for _, d := range decls {
		if !r.declIsPattern(d) {
			continue
		}
		names, ok := r.declBindingNames(d)
		if !ok {
			continue
		}
		for _, nn := range names {
			sym, ok := r.prog.SymbolAt(nn)
			if !ok || !r.moduleAssignPatternNames[sym] {
				continue
			}
			if name, ok := localName(r.prog.Text(nn)); ok {
				out[name] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// storeIntoHoistedPattern rewrites a destructuring statement's binds to store into the
// package vars its leaves were hoisted to. A leaf ordinarily binds with `:=`, and a
// defaulted one first declares `var name T` and then fills it; both would declare a fresh
// main local that shadows the package var, so the emitted Go would build and the function
// reading the name would still see an unset value. The `:=` becomes `=` and the redundant
// declaration is dropped.
//
// Only the statement's own top level is rewritten. A bind nested inside an if or a block
// is a fill into a name already declared above it, and the temporaries the expansion mints
// for a held source or a nested slot are main's alone, so both are left as they are.
func storeIntoHoistedPattern(stmts []ast.Stmt, hoisted map[string]bool) ([]ast.Stmt, error) {
	if len(hoisted) == 0 {
		return stmts, nil
	}
	out := make([]ast.Stmt, 0, len(stmts))
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.AssignStmt:
			if st.Tok != token.DEFINE {
				break
			}
			marked, total := countHoistedIdents(st.Lhs, hoisted)
			if marked == 0 {
				break
			}
			if marked != total {
				return nil, &NotYetLowerable{Reason: "a module destructuring bind that mixes a hoisted leaf with a main-local one is a later slice"}
			}
			st.Tok = token.ASSIGN
		case *ast.DeclStmt:
			gd, ok := st.Decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				break
			}
			marked, total := 0, 0
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, nm := range vs.Names {
					total++
					if hoisted[nm.Name] {
						marked++
					}
				}
			}
			if marked == 0 {
				break
			}
			if marked != total {
				return nil, &NotYetLowerable{Reason: "a module destructuring bind that mixes a hoisted leaf with a main-local one is a later slice"}
			}
			// The package var already declares the leaf, so the local declaration this fill
			// would make is dropped and the fill below assigns straight into it.
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// countHoistedIdents counts how many of a bind's targets are hoisted package vars and how
// many targets it has, so a caller can tell an all-hoisted bind from a mixed one. A blank
// target is not counted at all, since `_` is neither.
func countHoistedIdents(lhs []ast.Expr, hoisted map[string]bool) (marked, total int) {
	for _, e := range lhs {
		id, ok := e.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}
		total++
		if hoisted[id.Name] {
			marked++
		}
	}
	return marked, total
}
