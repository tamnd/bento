package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lands the general tagged-sum union of 05_type_lowering section 9, the
// representation for a union whose members are unlike types the code narrows with
// typeof, a discriminant, or in before using. The closed string-literal union
// (union.go) and the optional T | undefined (optionals.go) keep their own leaner
// forms; this is the fallback for the rest.
//
// A union of primitive arms lowers to a value struct: a small integer discriminant
// tag plus one inline field per arm, only the field matching the tag meaningful.
//
//	type NumOrStrTag uint8
//
//	const (
//		NumOrStrNum NumOrStrTag = iota
//		NumOrStrStr
//	)
//
//	type NumOrStr struct {
//		tag NumOrStrTag
//		num float64
//		str value.BStr
//	}
//
//	func NumOrStrOfNum(v float64) NumOrStr { return NumOrStr{tag: NumOrStrNum, num: v} }
//	func NumOrStrOfStr(v value.BStr) NumOrStr { return NumOrStr{tag: NumOrStrStr, str: v} }
//
// The struct is passed by value with every arm inline, so constructing one and
// reading a narrowed arm never touch the heap: construction is a struct literal,
// narrowing is a single integer compare on the tag, and a narrowed read is a
// direct field select. That is the whole reason to carry the tag rather than a
// boxed self-describing value, and it is what keeps a union faster than the tagged
// pointer a dynamic engine would carry for the same value.

// primArm describes one primitive arm a tagged-sum union can carry: the type flag
// that recognizes it, the struct field it stores in, the exported suffix its tag
// constant and constructor take, the typeof tag that narrows to it, and a rank so
// the arms of one union order the same way regardless of the order the checker
// lists the members. Only these primitives are inline arms; an object, array, or
// class arm needs the pointer-field form of a later slice.
type primArm struct {
	flag   frontend.TypeFlags
	field  string
	suffix string
	typeof string
	rank   int
}

var primArms = []primArm{
	{frontend.TypeNumber, "num", "Num", "number", 0},
	{frontend.TypeString, "str", "Str", "string", 1},
	{frontend.TypeBoolean, "bl", "Bool", "boolean", 2},
	{frontend.TypeBigInt, "big", "Big", "bigint", 3},
}

// unionArm is one arm of an interned union: the primitive descriptor it matched
// plus the Go type its field and constructor take, resolved once through typeExpr
// so the emitted field, the constructor parameter, and any bridge agree.
type unionArm struct {
	primArm
	goType ast.Expr
}

// unionInfo is the interned descriptor of one tagged-sum union: the Go type name,
// its discriminant tag type, and its arms in canonical order. The construction,
// narrowing, and emission paths all read it so the tag a constructor sets, the tag
// a typeof compares, and the field a narrowed read selects stay consistent.
type unionInfo struct {
	goName  string
	tagType string
	arms    []unionArm
}

// tagConst returns the discriminant constant name for an arm, the union name
// concatenated with the arm suffix (NumOrStrStr), unique across unions because the
// union name is.
func (u *unionInfo) tagConst(a unionArm) string { return u.goName + a.suffix }

// ctorName returns the wrapping constructor name for an arm (NumOrStrOfStr), the
// one function that sets both the tag and the matching field so the two never drift
// apart.
func (u *unionInfo) ctorName(a unionArm) string { return u.goName + "Of" + a.suffix }

// armForFlags returns the arm a single-primitive type selects, matching by the
// arm's recognizing flag. It is how a construction site picks the constructor for
// the value it is wrapping and how a narrowed read picks the field to select. A
// type that is not exactly one of the union's primitive arms returns false, so the
// caller keeps the whole union rather than guess an arm.
func (u *unionInfo) armForFlags(f frontend.TypeFlags) (unionArm, bool) {
	// A narrowed boolean is the true|false union the checker keeps, so its flags carry
	// the union bit alongside the boolean bit; matching on the arm bit rather than
	// rejecting any union lets that narrowing select the boolean arm. The whole
	// tagged-sum union carries only the union bit and none of the arm bits, so it
	// matches no arm here and the caller keeps the bare struct.
	for _, a := range u.arms {
		if f&a.flag != 0 {
			return a, true
		}
	}
	return unionArm{}, false
}

// isPrimitiveUnionType reports whether a type is a union every one of whose members
// is a distinct supported primitive (number, string, boolean, bigint), the shape
// the inline tagged-sum covers. A union with an object, array, class, null, or
// undefined member, or fewer than two members, is not this shape and lowers
// elsewhere or hands back.
func (r *Renderer) isPrimitiveUnionType(t frontend.Type) bool {
	if t.Flags&frontend.TypeUnion == 0 {
		return false
	}
	_, ok := r.primUnionArms(t)
	return ok
}

// primUnionArms classifies each member of a union to a primitive arm, returning the
// arms in canonical rank order. It returns false when any member is not a supported
// primitive, so the caller can tell a lowerable primitive union from one that still
// hands back. The checker expands a boolean member into its true and false literal
// members, so the two collapse to the single boolean arm; two members that map to
// any other arm (a string-literal enum) return false, leaving that shape to the
// string-enum lowering.
func (r *Renderer) primUnionArms(t frontend.Type) ([]primArm, bool) {
	members := r.prog.UnionMembers(t)
	if len(members) < 2 {
		return nil, false
	}
	seen := map[frontend.TypeFlags]bool{}
	arms := make([]primArm, 0, len(members))
	for _, m := range members {
		arm, ok := memberArm(m.Flags)
		if !ok {
			return nil, false
		}
		if seen[arm.flag] {
			// A repeated boolean arm is the true|false expansion collapsing back to
			// one boolean; a repeat of any other arm is a literal enum this path
			// leaves alone.
			if arm.flag == frontend.TypeBoolean {
				continue
			}
			return nil, false
		}
		seen[arm.flag] = true
		arms = append(arms, arm)
	}
	if len(arms) < 2 {
		return nil, false
	}
	sortArmsByRank(arms)
	return arms, true
}

// memberArm maps one union member's flags to its primitive arm. A member whose
// flags are exactly a base primitive is that arm; a boolean literal (the true or
// false the checker splits a boolean member into) carries the boolean bit and maps
// to the boolean arm. Every other member, a string or number literal or an object,
// is not an inline arm and returns false.
func memberArm(f frontend.TypeFlags) (primArm, bool) {
	for _, a := range primArms {
		if f == a.flag {
			return a, true
		}
	}
	if f&frontend.TypeBoolean != 0 {
		for _, a := range primArms {
			if a.flag == frontend.TypeBoolean {
				return a, true
			}
		}
	}
	return primArm{}, false
}

// sortArmsByRank orders arms by their fixed rank so the tag assignment and the
// generated name are independent of the order the checker lists the union members,
// the same order-independence the string-enum interner keeps by sorting the values.
func sortArmsByRank(arms []primArm) {
	for i := 1; i < len(arms); i++ {
		for j := i; j > 0 && arms[j-1].rank > arms[j].rank; j-- {
			arms[j-1], arms[j] = arms[j], arms[j-1]
		}
	}
}

// internUnion returns the interned descriptor for a primitive tagged-sum union,
// generating its Go type the first time it sees the shape and reusing it after,
// keyed on the structural signature so two structurally equal unions share one
// type. A union outside the primitive-arm subset returns a NotYetLowerable so the
// type renderer hands the unit back rather than emit a struct for a shape the
// construction and narrowing paths cannot serve yet.
func (r *Renderer) internUnion(t frontend.Type) (*unionInfo, error) {
	sig := structuralKey(r.prog, t, map[int]int{})
	if info, ok := r.unionBySig[sig]; ok {
		return info, nil
	}

	prims, ok := r.primUnionArms(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "union with a non-primitive member needs the object-arm tagged sum, a later slice"}
	}

	suffixes := make([]string, len(prims))
	for i, a := range prims {
		suffixes[i] = a.suffix
	}
	goName := r.decls.reserveName(strings.Join(suffixes, "Or"))

	arms := make([]unionArm, len(prims))
	for i, a := range prims {
		gt, err := r.typeExpr(memberType(r.prog, t, a.flag))
		if err != nil {
			return nil, err
		}
		arms[i] = unionArm{primArm: a, goType: gt}
	}

	info := &unionInfo{goName: goName, tagType: goName + "Tag", arms: arms}
	r.unions = append(r.unions, info)
	r.unionBySig[sig] = info
	return info, nil
}

// memberType returns the union member whose flags match a primitive arm, so the
// arm's Go field and constructor take that member's own lowered type rather than a
// synthesized one, which keeps a branded or aliased primitive arm lowering to the
// same Go type it does everywhere else.
func memberType(prog *frontend.Program, t frontend.Type, flag frontend.TypeFlags) frontend.Type {
	for _, m := range prog.UnionMembers(t) {
		if m.Flags == flag {
			return m
		}
	}
	return frontend.Type{Flags: flag}
}

// unionInfoOf returns the interned descriptor for a type when it is a tagged-sum
// union already interned, a pure lookup used by the construction and narrowing
// paths, which run after the type renderer has interned every param, return, and
// binding type. A type that is not an interned union returns false.
func (r *Renderer) unionInfoOf(t frontend.Type) (*unionInfo, bool) {
	if t.Flags&frontend.TypeUnion == 0 {
		return nil, false
	}
	info, ok := r.unionBySig[structuralKey(r.prog, t, map[int]int{})]
	return info, ok
}

// unionInfoOrIntern returns the descriptor for a primitive union, interning it if
// the type renderer has not reached it yet. The local pre-pass uses it so a union
// named only inside one body (a const whose union type appears nowhere in a
// signature) is still interned before its reads are lowered, and interning is
// idempotent, so calling it here and again at a use site emits one type.
func (r *Renderer) unionInfoOrIntern(t frontend.Type) (*unionInfo, bool) {
	if !r.isPrimitiveUnionType(t) {
		return nil, false
	}
	info, err := r.internUnion(t)
	if err != nil {
		return nil, false
	}
	return info, true
}

// wrapToUnion wraps a value flowing into a tagged-sum union slot in the arm
// constructor for its type, the construction side of section 9: assigning a member
// value into a union slot is always this wrap, never a bare assignment, so the tag
// stays consistent with the payload. It returns (expr, false, nil) when the target
// is not a tagged-sum union or the source is already the same union (a union bound
// to a union passes through unwrapped), leaving the caller on its existing path. A
// source whose type is not one of the union's arms hands back rather than guess.
func (r *Renderer) wrapToUnion(expr ast.Expr, src frontend.Node, target frontend.Type) (ast.Expr, bool, error) {
	info, ok := r.unionInfoOrIntern(target)
	if !ok {
		return expr, false, nil
	}
	if other, ok := r.unionInfoOf(r.prog.TypeAt(src)); ok && other == info {
		return expr, false, nil
	}
	arm, ok := info.armForFlags(r.primitiveFlags(src))
	if !ok {
		return nil, false, &NotYetLowerable{Reason: "constructing this union from its source type is a later slice"}
	}
	return &ast.CallExpr{Fun: ident(info.ctorName(arm)), Args: []ast.Expr{expr}}, true, nil
}

// narrowedUnionRead lowers a reference to a union-typed local the checker narrowed
// to one arm at this use into a read of that arm's field, the read side of section
// 9: inside a branch the flow analysis narrowed the union to a member, so the field
// is touched directly with no runtime test beyond the tag switch already taken. A
// reference where the type is still the whole union returns false so the caller
// keeps the bare struct, which is what an assignment or a pass-through of the union
// wants.
func (r *Renderer) narrowedUnionRead(name string, n frontend.Node) (ast.Expr, bool) {
	info, ok := r.unionLocals[name]
	if !ok {
		return nil, false
	}
	arm, ok := info.armForFlags(r.primitiveFlags(n))
	if !ok {
		return nil, false
	}
	return &ast.SelectorExpr{X: ident(name), Sel: ident(arm.field)}, true
}

// typeofUnionCompare lowers a typeof test on a tagged-sum union against a string
// literal to a discriminant compare, the typeof-narrowing of section 9: typeof x
// === "string" on a string | number lowers to comparing the tag rather than
// building the "string" tag and matching it. One operand must be typeof of a union
// local and the other a string literal that names one of the union's arms. It
// returns false for any other shape so the caller falls through to the value
// compare, and negates the result for !==.
func (r *Renderer) typeofUnionCompare(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	if opText != "===" && opText != "!==" {
		return nil, false, nil
	}
	operand, lit, ok := r.typeofOperandAndLiteral(left, right)
	if !ok {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(operand))
	if !ok {
		return nil, false, nil
	}
	info, ok := r.unionLocals[name]
	if !ok {
		return nil, false, nil
	}
	var arm unionArm
	found := false
	for _, a := range info.arms {
		if a.typeof == lit {
			arm, found = a, true
			break
		}
	}
	if !found {
		return nil, false, nil
	}
	cmp := &ast.BinaryExpr{
		X:  &ast.SelectorExpr{X: ident(name), Sel: ident("tag")},
		Op: token.EQL,
		Y:  ident(info.tagConst(arm)),
	}
	if opText == "!==" {
		cmp.Op = token.NEQ
	}
	return cmp, true, nil
}

// typeofOperandAndLiteral picks the typeof operand and the string-literal tag out
// of a comparison's two sides, in either order, and returns the operand node and
// the literal's value. It returns false unless exactly one side is typeof x and the
// other is a string literal, so a typeof against a non-literal or two typeofs do
// not match.
func (r *Renderer) typeofOperandAndLiteral(a, b frontend.Node) (frontend.Node, string, bool) {
	if r.isTypeofExpr(a) {
		if lit, ok := r.stringLiteralValue(b); ok {
			return r.prog.Children(a)[0], lit, true
		}
	}
	if r.isTypeofExpr(b) {
		if lit, ok := r.stringLiteralValue(a); ok {
			return r.prog.Children(b)[0], lit, true
		}
	}
	return nil, "", false
}

// stringLiteralValue returns the value of a string-literal node, the "string" or
// "number" a typeof test compares against. It reads the checker's literal type so a
// const holding the literal is seen the same as the bare literal, and returns false
// for a non-string-literal operand.
func (r *Renderer) stringLiteralValue(n frontend.Node) (string, bool) {
	lit, ok := r.prog.LiteralValue(r.prog.TypeAt(n))
	if !ok || lit.Kind != frontend.LiteralString {
		return "", false
	}
	return lit.Str, true
}

// unionLocalsOf collects the parameter and local binding names of one body whose
// declared type is an interned tagged-sum union, so a read of one narrowed to an
// arm reads that arm's field. It scans the signature parameters and then the body
// the way optLocalsOf does, dropping any name declared in more than one scope since
// the flat name set cannot tell two scopes apart and a wrong field read would be
// unsound. A nil map means no union binding to read out of.
func (r *Renderer) unionLocalsOf(params []frontend.Param, body []frontend.Node) map[string]*unionInfo {
	out := map[string]*unionInfo{}
	declCount := map[string]int{}
	for _, p := range params {
		name, ok := localName(p.Name)
		if !ok {
			continue
		}
		declCount[name]++
		if info, ok := r.unionInfoOrIntern(p.Type); ok {
			out[name] = info
		}
	}
	for _, n := range body {
		r.collectUnionDecls(n, out, declCount)
	}
	for name, c := range declCount {
		if c != 1 {
			delete(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectUnionDecls walks one node, recording each variable declaration whose name
// is typed as a tagged-sum union, and recurses into its children so a binding in a
// nested block or loop is seen. It counts declarations per name alongside so
// unionLocalsOf can drop a name declared in more than one scope, the same guard the
// optional pre-pass keeps.
func (r *Renderer) collectUnionDecls(n frontend.Node, out map[string]*unionInfo, declCount map[string]int) {
	if n.Kind() == frontend.NodeVariableDeclaration {
		kids := r.prog.Children(n)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			if name, ok := localName(r.prog.Text(kids[0])); ok {
				declCount[name]++
				if info, ok := r.unionInfoOrIntern(r.prog.TypeAt(kids[0])); ok {
					out[name] = info
				}
			}
		}
	}
	for _, c := range r.prog.Children(n) {
		r.collectUnionDecls(c, out, declCount)
	}
}

// renderUnions emits the interned tagged-sum unions as package-level declarations,
// each as its discriminant tag type, the const block that names the tags, the sum
// struct, and one wrapping constructor per arm, in first-seen order so the output
// is deterministic. The program assembler splices these in beside the interned
// structs and enums, before the code that constructs and narrows them.
func (r *Renderer) renderUnions() []ast.Decl {
	var out []ast.Decl
	for _, info := range r.unions {
		out = append(out, unionTagType(info), unionTagConsts(info), unionStruct(info))
		for _, a := range info.arms {
			out = append(out, unionCtor(info, a))
		}
	}
	return out
}

// unionTagType builds the discriminant type declaration, an unsigned integer the
// tag constants run over.
func unionTagType(info *unionInfo) ast.Decl {
	return &ast.GenDecl{
		Tok:   token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{Name: ident(info.tagType), Type: ident("uint8")}},
	}
}

// unionTagConsts builds the const block naming one tag per arm, the first at iota
// so the rest follow in arm order, matching the string-enum const shape.
func unionTagConsts(info *unionInfo) ast.Decl {
	decl := &ast.GenDecl{Tok: token.CONST, Lparen: token.Pos(1), Rparen: token.Pos(1)}
	for i, a := range info.arms {
		spec := &ast.ValueSpec{Names: []*ast.Ident{ident(info.tagConst(a))}}
		if i == 0 {
			spec.Type = ident(info.tagType)
			spec.Values = []ast.Expr{ident("iota")}
		}
		decl.Specs = append(decl.Specs, spec)
	}
	return decl
}

// unionStruct builds the sum struct: the discriminant tag first, then one inline
// field per arm holding that arm's value when the tag selects it.
func unionStruct(info *unionInfo) ast.Decl {
	fields := &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{ident("tag")}, Type: ident(info.tagType)},
	}}
	for _, a := range info.arms {
		fields.List = append(fields.List, &ast.Field{Names: []*ast.Ident{ident(a.field)}, Type: a.goType})
	}
	return &ast.GenDecl{
		Tok:   token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{Name: ident(info.goName), Type: &ast.StructType{Fields: fields}}},
	}
}

// unionCtor builds one arm's wrapping constructor, which sets the tag and the
// matching field in a single struct literal so a construction never leaves the two
// out of step.
func unionCtor(info *unionInfo, a unionArm) ast.Decl {
	return &ast.FuncDecl{
		Name: ident(info.ctorName(a)),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("v")}, Type: a.goType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ident(info.goName)}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.CompositeLit{
				Type: ident(info.goName),
				Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ident("tag"), Value: ident(info.tagConst(a))},
					&ast.KeyValueExpr{Key: ident(a.field), Value: ident("v")},
				},
			},
		}}}},
	}
}
