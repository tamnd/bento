package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A destructuring pattern whose elements carry no static type reads its source as a
// boxed value.Value rather than a Go struct or slice. This is the JS-as-TS shape the
// checker flags with "Binding element 'X' implicitly has an 'any' type" (7031): the
// pattern has no annotation, so every element resolves to any, and there is no fixed
// shape to intern to a field or an index. The typed binder cannot serve it, since it
// would read `.Field` off a struct the argument does not carry, so an untyped pattern
// binds each element through the same dynamic protocol a dynamic member or index read
// takes: an object property through Get. Each bound name is a value.Value the body
// then reads on the dynamic path.

// dynamicParamSlot reports whether a parameter is a destructured pattern with no
// static shape, the JS-as-TS parameter the checker flags 7031 (or the same pattern
// annotated any). Such a parameter arrives as one boxed value.Value slot rather than
// a Go struct, and its bound names read out of it through the dynamic protocol. The
// name is synthesized (__0, __1, and so on), which only a destructured parameter
// carries, so a plainly named any parameter is left alone; the type is any, or an
// object every one of whose properties is dynamic, which is the shape a fully untyped
// pattern infers. A pattern with a static leaf keeps the typed binder, which reads
// that leaf through the field or index its shape declares.
func (r *Renderer) dynamicParamSlot(p frontend.Param) bool {
	if !isSynthParamName(p.Name) {
		return false
	}
	if p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return true
	}
	if p.Type.Flags&frontend.TypeObject == 0 {
		return false
	}
	props := r.prog.Properties(p.Type)
	if len(props) == 0 {
		return false
	}
	// An untyped array pattern interns its positions as numeric-named properties, every
	// one dynamic; a typed tuple types those positions to real leaves, so a numeric
	// property that is not dynamic means the typed binder still serves. The array's own
	// methods carry object types, so they are read past here rather than counted against
	// the pattern's dynamic shape.
	numeric := false
	for _, pr := range props {
		if !isNumericName(pr.Name) {
			continue
		}
		numeric = true
		if pr.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return false
		}
	}
	if numeric {
		return true
	}
	// An untyped object pattern infers an anonymous object every one of whose properties
	// is dynamic.
	for _, pr := range props {
		if pr.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return false
		}
	}
	return true
}

// isNumericName reports whether a property name is an all-digit index position, the
// key an array pattern interns for each of its slots. It tells an array pattern's
// positional properties from an object pattern's named ones.
func isNumericName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isSynthParamName reports whether a parameter name is the synthesized __N the
// frontend gives a destructured parameter, which has no source identifier of its own.
// It is the tell that a parameter binds a pattern rather than a plain name.
func isSynthParamName(name string) bool {
	if !strings.HasPrefix(name, "__") || len(name) == 2 {
		return false
	}
	for _, c := range name[2:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// bindDynamicPattern binds a pattern against a dynamic receiver, reading each element
// through the boxed value's dynamic protocol and binding it as a value.Value. This
// first slice serves an object pattern of shorthand names, which is the bulk of the
// untyped-pattern cluster; an array pattern, a rename, a default, and a nested pattern
// are later slices on this path and hand back. tok is DEFINE for a declaration or
// parameter binding.
func (r *Renderer) bindDynamicPattern(pat frontend.Node, recv ast.Expr, tok token.Token) ([]ast.Stmt, error) {
	txt := strings.TrimSpace(r.prog.Text(pat))
	if strings.HasPrefix(txt, "{") {
		return r.bindDynamicObject(pat, recv, tok)
	}
	if strings.HasPrefix(txt, "[") {
		return r.bindDynamicArray(pat, recv, tok)
	}
	return nil, &NotYetLowerable{Reason: "an untyped destructuring pattern that is neither an object nor an array is a later slice"}
}

// bindDynamicArray binds an array pattern against a dynamic receiver. Each fixed
// position reads through GetIndex and binds its name as a value.Value, the same indexed
// read a dynamic element access lowers to, so an untyped array pattern reads its slots
// the way a typed one reads them through AtI. This first slice serves the plain-name
// positions the bulk of the untyped-pattern cluster uses; a default, a rest, a hole, and
// a nested element are later slices on this path and hand back, so the pattern lowers the
// simple form and declines the rest honestly.
func (r *Renderer) bindDynamicArray(pat frontend.Node, recv ast.Expr, tok token.Token) ([]ast.Stmt, error) {
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array destructuring pattern binds nothing"}
	}
	if _, _, hasRest, err := r.splitArrayRest(elems); err != nil {
		return nil, err
	} else if hasRest {
		return nil, &NotYetLowerable{Reason: "a rest on an untyped array destructuring pattern is a later slice"}
	}
	r.requireImport(valuePkg)
	var out []ast.Stmt
	for i, el := range elems {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, err
		}
		if info.nested != nil {
			return nil, &NotYetLowerable{Reason: "a nested pattern on an untyped array destructuring element is a later slice"}
		}
		if info.hasDefault {
			return nil, &NotYetLowerable{Reason: "a default on an untyped array destructuring element is a later slice"}
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "an untyped destructuring target is not a Go identifier"}
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{dynIndex(recv, i)}})
	}
	return out, nil
}

// bindDynamicObject binds an object pattern against a dynamic receiver. A shorthand
// property reads through Get by its name and binds the same name as a value.Value.
// Each read is emitted unconditionally, the same as the typed binder: an orphaned name
// is blanked by the shared unused-binding pass, so a name the body never reads needs no
// special case here. A rename, default, computed key, rest, or nested element is a
// later slice on this path and hands back, so the untyped object pattern lowers the
// shorthand form the bulk of the cluster uses and declines the rest honestly.
func (r *Renderer) bindDynamicObject(pat frontend.Node, recv ast.Expr, tok token.Token) ([]ast.Stmt, error) {
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty object destructuring pattern binds nothing"}
	}
	r.requireImport(valuePkg)
	var out []ast.Stmt
	for _, el := range elems {
		if _, _, ok := r.objectNestedElem(el); ok {
			return nil, &NotYetLowerable{Reason: "a nested pattern on an untyped object destructuring element is a later slice"}
		}
		info, err := r.classifyObjectElem(el)
		if err != nil {
			return nil, err
		}
		if info.hasDefault {
			return nil, &NotYetLowerable{Reason: "a default on an untyped object destructuring element is a later slice"}
		}
		prop := strings.TrimSpace(r.prog.Text(info.nameNode))
		name, ok := localName(r.prog.Text(info.bindNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "an untyped destructuring target is not a Go identifier"}
		}
		// A rename ({a: b}) reads a different name than it binds; that is a later slice on
		// this path, so a shorthand alone reads and binds the same name here.
		if prop != name {
			return nil, &NotYetLowerable{Reason: "a rename on an untyped object destructuring element is a later slice"}
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{dynGet(recv, prop)}})
	}
	return out, nil
}

// dynGet reads a property off a dynamic receiver, recv.Get(value.FromGoString("prop")),
// the same boxed property read a dynamic member access lowers to.
func dynGet(recv ast.Expr, prop string) ast.Expr {
	key := &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(prop)}}}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{key}}
}

// dynIndex reads a position off a dynamic receiver, recv.GetIndex(i), the same boxed
// indexed read a dynamic element access lowers to. GetIndex takes a float64, and an
// untyped integer constant converts to it, so the position is emitted as an int literal.
func dynIndex(recv ast.Expr, i int) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetIndex")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}}}
}
