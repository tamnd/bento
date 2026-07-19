package lower

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
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
//
// undefined and null are tag-only arms: each is a single sentinel value carrying no
// payload, so it needs a tag to tell it apart from the value arms but no struct
// field to store, no constructor parameter, and no Go type. Their typeof strings are
// the ones JavaScript reports, "undefined" for undefined and "object" for null, so a
// typeof narrowing and the TypeOf method answer them from the tag like any other arm.
type primArm struct {
	flag    frontend.TypeFlags
	field   string
	suffix  string
	typeof  string
	rank    int
	tagOnly bool
}

var primArms = []primArm{
	{flag: frontend.TypeNumber, field: "num", suffix: "Num", typeof: "number", rank: 0},
	{flag: frontend.TypeString, field: "str", suffix: "Str", typeof: "string", rank: 1},
	{flag: frontend.TypeBoolean, field: "bl", suffix: "Bool", typeof: "boolean", rank: 2},
	{flag: frontend.TypeBigInt, field: "big", suffix: "Big", typeof: "bigint", rank: 3},
	{flag: frontend.TypeUndefined, suffix: "Undef", typeof: "undefined", rank: 4, tagOnly: true},
	{flag: frontend.TypeNull, suffix: "Null", typeof: "object", rank: 5, tagOnly: true},
}

// singletonName returns the value-package singleton a tag-only arm's narrowed read
// evaluates to, value.Undefined for the undefined arm and value.Null for the null
// arm, the same singletons the bare undefined and null literals lower to, so a
// reference the checker narrowed to the sentinel reads as that one value. It is
// meaningful only for a tag-only arm.
func (a primArm) singletonName() string {
	if a.flag == frontend.TypeNull {
		return "Null"
	}
	return "Undefined"
}

// unionArm is one arm of an interned union: the primitive descriptor it matched
// plus the Go type its field and constructor take, resolved once through typeExpr
// so the emitted field, the constructor parameter, and any bridge agree. An object
// arm sets isObject and carries the discriminant literal that selects it and the
// structural key of its member type, so a narrowed read and a discriminant compare
// find the arm without a primitive flag.
type unionArm struct {
	primArm
	goType    ast.Expr
	isObject  bool
	disc      string          // the discriminant literal value an object arm narrows on ("circle")
	memberSig string          // the structural key of an object arm's member type
	props     map[string]bool // the property names an object arm's member carries, for in narrowing
}

// unionInfo is the interned descriptor of one tagged-sum union: the Go type name,
// its discriminant tag type, and its arms in canonical order. The construction,
// narrowing, and emission paths all read it so the tag a constructor sets, the tag
// a typeof compares, and the field a narrowed read selects stay consistent.
type unionInfo struct {
	goName  string
	tagType string
	arms    []unionArm
	// disc is the discriminant property name of an object union ("kind"), the
	// property whose string-literal value differs per arm. It is empty for a
	// primitive union, which narrows on typeof rather than a property.
	disc string
	// needsTypeOf records that a bare typeof over a value of this union was lowered,
	// so the renderer emits the TypeOf method that switches the tag to its typeof
	// string. It stays false for a union only ever narrowed by a typeof compare,
	// which folds to a tag test and never calls the method.
	needsTypeOf bool
	// needsToBoolean records that a value of this union stood in boolean position, so
	// the renderer emits the ToBoolean method that switches the tag to its arm's
	// JavaScript truthiness. It stays false for a union never tested for truth.
	needsToBoolean bool
	// needsToString records that a value of this union was coerced to a string, so the
	// renderer emits the ToString method that switches the tag to its arm's JavaScript
	// string form. It stays false for a union never stringified.
	needsToString bool
}

// armByDisc returns the object arm a discriminant literal selects, so a compare
// s.kind === "circle" or a switch case "circle" maps to the arm's tag. It returns
// false when no arm carries that literal.
func (u *unionInfo) armByDisc(v string) (unionArm, bool) {
	for _, a := range u.arms {
		if a.isObject && a.disc == v {
			return a, true
		}
	}
	return unionArm{}, false
}

// armByMemberSig returns the object arm whose member type has this structural key,
// the match a narrowed read and a construction use to pick the arm from the object
// value's own type rather than a discriminant. It returns false when no arm's member
// matches.
func (u *unionInfo) armByMemberSig(sig string) (unionArm, bool) {
	for _, a := range u.arms {
		if a.isObject && a.memberSig == sig {
			return a, true
		}
	}
	return unionArm{}, false
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
	// tagged-sum union carries only the union bit and none of the value-arm bits, so
	// it matches no value arm here and the caller keeps the bare struct.
	//
	// A tag-only sentinel arm (undefined, null) matches only when the flags are
	// exactly that sentinel, the shape a reference narrowed to the sentinel carries.
	// The whole union's own flags may include the undefined or null bit alongside the
	// union bit, so a subset test would misread a whole-union reference as the sentinel
	// arm; the exact match keeps it to a genuinely narrowed sentinel.
	for _, a := range u.arms {
		if a.tagOnly {
			if f == a.flag {
				return a, true
			}
			continue
		}
		if f&a.flag != 0 {
			return a, true
		}
	}
	return unionArm{}, false
}

// undefArm returns the union's tag-only undefined arm, the arm an absent optional
// property constructs. A union without an undefined member (a required union, or one
// whose only sentinel is null) returns false.
func (u *unionInfo) undefArm() (unionArm, bool) {
	for _, a := range u.arms {
		if a.flag == frontend.TypeUndefined {
			return a, true
		}
	}
	return unionArm{}, false
}

// optionalUnionInfo returns the interned tagged-sum descriptor for an optional
// property whose type is a multi-member union rather than the two-member T | undefined
// shape (which lowers to value.Opt). The union must carry an undefined arm, because
// that arm is how an absent property is constructed and read: a tag?: number | string
// types as number | string | undefined, and the undefined arm stands for the omitted
// member. A property that is not optional, that is the value.Opt shape, or whose union
// does not intern (an object-mixed union) returns false so the caller keeps the
// value.Opt path or hands back.
func (r *Renderer) optionalUnionInfo(p frontend.Property) (*unionInfo, bool) {
	if !p.Optional || r.isOptionalType(p.Type) {
		return nil, false
	}
	if p.Type.Flags&frontend.TypeUnion == 0 {
		return nil, false
	}
	hasUndef := false
	for _, m := range r.prog.UnionMembers(p.Type) {
		if m.Flags == frontend.TypeUndefined {
			hasUndef = true
			break
		}
	}
	if !hasUndef {
		return nil, false
	}
	info, err := r.internUnion(p.Type)
	if err != nil {
		return nil, false
	}
	if _, ok := info.undefArm(); !ok {
		return nil, false
	}
	return info, true
}

// unionUndefValue is the no-argument undefined-arm constructor call, the value an
// omitted or explicitly-undefined optional union property takes in a struct literal.
func (r *Renderer) unionUndefValue(info *unionInfo) ast.Expr {
	arm, _ := info.undefArm()
	return &ast.CallExpr{Fun: ident(info.ctorName(arm))}
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
			// A repeat is either the boolean true|false expansion collapsing back to one
			// boolean, or two literals of the same base widened to one arm (the "" and "x"
			// inside "" | "x" | 0 | 1, or a pure "a" | "b"). The checker dedupes identical
			// members, so a non-boolean repeat is only ever same-base literals, which
			// collapse to their single arm the way the booleans do. A shape that collapses
			// to fewer than two value arms (a pure single-base literal enum) still fails the
			// valueArms check below and falls to the string-enum lowering, so only a
			// genuinely mixed union reaches the inline tagged sum.
			continue
		}
		seen[arm.flag] = true
		arms = append(arms, arm)
	}
	// A tag-only sentinel arm rides alongside two or more value arms, or beside a single
	// value arm when a null sentinel is among its companions. A T | undefined optional
	// keeps its own value.Opt path, which the optional pre-pass owns, so a single value
	// arm beside only the undefined sentinel still hands back here; but null has no
	// optional wrapper of its own, so the moment a null sentinel appears the union interns
	// as a tagged sum, a value arm plus the sentinel tags, rather than fall through to the
	// object-union path and hand back. That covers both T | null and T | null | undefined.
	valueArms := 0
	for _, a := range arms {
		if !a.tagOnly {
			valueArms++
		}
	}
	if valueArms < 2 && !nullSentinelUnion(arms, valueArms) {
		return nil, false
	}
	sortArmsByRank(arms)
	return arms, true
}

// nullSentinelUnion reports whether arms is a single value arm whose tag-only companions
// include the null sentinel, the T | null or T | null | undefined shape. Null has no
// value.Opt wrapper of its own, so its presence forces the tagged-sum representation; a
// single value arm beside only the undefined sentinel (T | undefined) has that wrapper
// and stays with the optional pre-pass, so it returns false here. Any tag-only companion
// other than null or undefined is unexpected for a single-value union and also returns
// false, leaving it to hand back.
func nullSentinelUnion(arms []primArm, valueArms int) bool {
	if valueArms != 1 {
		return false
	}
	hasNull := false
	for _, a := range arms {
		if !a.tagOnly {
			continue
		}
		switch a.flag {
		case frontend.TypeNull:
			hasNull = true
		case frontend.TypeUndefined:
		default:
			return false
		}
	}
	return hasNull
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
	// A literal member (a string, number, or bigint literal type like "a", 1, or 1n)
	// carries the TypeLiteral bit alongside its base primitive, so it fails the exact
	// base match above; it widens to that base arm so a mixed union of unlike literals,
	// "a" | 1, interns the same NumOrStr a string | number does. This is sound because
	// the value flowing into the arm is the plain boxed primitive the literal produces,
	// value.BStr or float64, so the tagged sum carries and narrows it the ordinary way,
	// the literal-ness being a static fact with no runtime representation. Two literals
	// of one base ("a" | "b") both widen to the same arm and so collide in primUnionArms,
	// which falls that shape to the string-enum lowering, so only a genuinely mixed union
	// reaches an inline tagged sum here.
	if f&frontend.TypeLiteral != 0 {
		const litBase = frontend.TypeNumber | frontend.TypeString | frontend.TypeBigInt
		for _, a := range primArms {
			if a.flag&litBase != 0 && f&a.flag != 0 {
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

// internUnion returns the interned descriptor for a tagged-sum union, generating
// its Go type the first time it sees the shape and reusing it after, keyed on the
// structural signature so two structurally equal unions share one type. It covers
// two shapes: a union of unlike primitives, whose arms are inline value fields
// narrowed on typeof, and a discriminated union of objects sharing a string-literal
// discriminant property, whose arms are pointer fields narrowed on that property. A
// union that is neither returns a NotYetLowerable so the type renderer hands the
// unit back rather than emit a struct for a shape the construction and narrowing
// paths cannot serve yet.
func (r *Renderer) internUnion(t frontend.Type) (*unionInfo, error) {
	sig := structuralKey(r.prog, t, map[int]int{})
	if info, ok := r.unionBySig[sig]; ok {
		return info, nil
	}

	prims, ok := r.primUnionArms(t)
	if !ok {
		return r.internObjectUnion(t, sig)
	}

	suffixes := make([]string, len(prims))
	for i, a := range prims {
		suffixes[i] = a.suffix
	}
	goName := r.decls.reserveName(strings.Join(suffixes, "Or"))

	arms := make([]unionArm, len(prims))
	for i, a := range prims {
		if a.tagOnly {
			// A sentinel arm has no payload, so it carries no Go type and no field; only
			// its tag distinguishes it.
			arms[i] = unionArm{primArm: a}
			continue
		}
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

// internObjectUnion interns a discriminated union of objects: every member is an
// object type, and they share one property whose value is a distinct string literal
// per member, the discriminant. Each arm becomes a pointer field to the member's
// interned struct, and the discriminant literal names the tag, the constructor, and
// the field. A union that is not this shape, an object member without a common
// discriminant or a discriminant whose value is not a Go-identifier-safe string,
// hands back so the type renderer routes the unit to the interpreter.
func (r *Renderer) internObjectUnion(t frontend.Type, sig string) (*unionInfo, error) {
	members := r.prog.UnionMembers(t)
	if len(members) < 2 {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "union with a non-primitive member needs the object-arm tagged sum, a later slice"}
	}
	// A single plain-record object member beside a null sentinel is the nullable-object
	// shape, the first slice of the mixed object and non-object union: the object lowers
	// to a pointer field, identity-preserving the way an ordinary object binding is, and
	// each sentinel is a tag-only arm, so no discriminant is needed to tell the one object
	// arm from the sentinels. Null forces the tagged sum the way it does for a primitive
	// value arm (nullSentinelUnion), while a bare object | undefined stays with the
	// optional pre-pass and its value.Opt. The object arm must be a plain record: a
	// built-in with its own runtime representation (an array or RegExpExecArray, a Map or
	// Set, a class instance) carries behavior a plain struct copy would drop, so it stays
	// the dynamic value.Value the union falls back to below rather than a miscompiled
	// tagged sum. A primitive (non-sentinel) member still hands back below, and two or
	// more object members beside a sentinel wait on the discriminated form.
	if obj, sentinels, ok := nullableObjectMembers(members); ok && r.isPlainRecordType(obj) {
		return r.internNullableObject(t, sig, obj, sentinels)
	}
	for _, m := range members {
		if m.Flags&frontend.TypeObject == 0 || m.Flags&frontend.TypeUnion != 0 {
			return nil, &NotYetLowerable{Flags: t.Flags, Reason: "union mixing object and non-object members is a later slice"}
		}
	}
	disc, values, ok := r.discriminant(members)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "union of objects without a shared string-literal discriminant is a later slice"}
	}

	arms := make([]unionArm, len(members))
	suffixes := make([]string, len(members))
	for i, m := range members {
		suffix, ok := exportedField(values[i])
		if !ok {
			return nil, &NotYetLowerable{Flags: t.Flags, Reason: "a discriminant value that is not a Go identifier is a later slice"}
		}
		gt, err := r.renderObject(m)
		if err != nil {
			return nil, err
		}
		props := map[string]bool{}
		for _, p := range r.prog.Properties(m) {
			props[p.Name] = true
		}
		arms[i] = unionArm{
			primArm:   primArm{field: unexportedName(suffix), suffix: suffix},
			goType:    gt,
			isObject:  true,
			disc:      values[i],
			memberSig: structuralKey(r.prog, m, map[int]int{}),
			props:     props,
		}
		suffixes[i] = suffix
	}
	goName := r.decls.reserveName(strings.Join(suffixes, "Or"))
	info := &unionInfo{goName: goName, tagType: goName + "Tag", arms: arms, disc: disc}
	r.unions = append(r.unions, info)
	r.unionBySig[sig] = info
	return info, nil
}

// nullableObjectMembers reports whether a union's members are exactly one object type
// beside one or more null or undefined sentinels, with at least a null among them, the
// nullable-object shape internNullableObject lowers. It returns the object member and
// the sentinel members. A union with no object member, more than one object member, a
// non-sentinel non-object member, or only an undefined sentinel (which the optional
// pre-pass owns as a value.Opt) is not this shape and returns false, leaving it to the
// discriminated-object path or the handback. Requiring null mirrors the primitive
// nullSentinelUnion rule: null has no value.Opt wrapper of its own, so its presence is
// what forces the tagged-sum representation here.
func nullableObjectMembers(members []frontend.Type) (frontend.Type, []frontend.Type, bool) {
	var obj frontend.Type
	objects := 0
	var sentinels []frontend.Type
	hasNull := false
	for _, m := range members {
		switch {
		case m.Flags == frontend.TypeNull:
			sentinels = append(sentinels, m)
			hasNull = true
		case m.Flags == frontend.TypeUndefined:
			sentinels = append(sentinels, m)
		case m.Flags&frontend.TypeObject != 0 && m.Flags&frontend.TypeUnion == 0:
			obj = m
			objects++
		default:
			return frontend.Type{}, nil, false
		}
	}
	if objects != 1 || !hasNull {
		return frontend.Type{}, nil, false
	}
	return obj, sentinels, true
}

// isPlainRecordType reports whether an object type is a plain data record, the shape
// renderObject faithfully interns to a named Go struct held by pointer, as opposed to a
// built-in object with its own runtime representation. It is a frontend-only check, so
// it never renders the type and so never registers the broken struct that interning a
// non-plain shape (RegExpExecArray, whose Array-inherited members do not form a fixed
// field set) would leave in the decl set. A class instance carries methods and identity
// a struct copy would drop; an array-like type (Array and the lib interfaces that extend
// it) has an element sequence, not fixed fields; a callable object has a call or
// construct signature; and a Map, Set, RegExp, typed array, Date, or Temporal type is a
// method bundle whose behavior lives in its methods, so a property that is itself
// callable (a method) or a property named by a non-identifier disqualifies the shape.
// What remains is a record of plain data fields, the only object arm the nullable-object
// tagged sum lowers in this slice; every other shape stays the dynamic value.Value the
// union already falls back to.
func (r *Renderer) isPlainRecordType(t frontend.Type) bool {
	if _, ok := r.classOfType(t); ok {
		return false
	}
	if _, ok := r.prog.ElementType(t); ok {
		return false
	}
	if call, ctor := r.prog.Signatures(t); len(call) > 0 || len(ctor) > 0 {
		return false
	}
	for _, p := range r.prog.Properties(t) {
		if _, ok := exportedField(p.Name); !ok {
			return false
		}
		if call, _ := r.prog.Signatures(p.Type); len(call) > 0 {
			return false
		}
	}
	return true
}

// sentinelArm returns the tag-only primitive arm a null or undefined sentinel member
// maps to, reusing the primArms descriptor so the arm's typeof string, singleton, and
// tag-only flag stay the ones the primitive-union path already uses. A flag that is not
// exactly a sentinel returns false.
func sentinelArm(f frontend.TypeFlags) (primArm, bool) {
	for _, a := range primArms {
		if a.tagOnly && a.flag == f {
			return a, true
		}
	}
	return primArm{}, false
}

// internNullableObject interns a union of one object arm and one or more null or
// undefined sentinels as a tagged sum: the object arm is a pointer field, so a value of
// the union carries the same reference an ordinary object binding does and object
// identity survives the box, and each sentinel is a tag-only arm distinguished by the
// tag alone. The object arm carries typeof "object", the string JavaScript reports for
// both an object and null, so a bare typeof over the union answers correctly and the
// typeof-narrowing guard sees the object and a null sentinel share it. No discriminant
// is needed, since one object arm is selected by not being a sentinel.
func (r *Renderer) internNullableObject(t frontend.Type, sig string, obj frontend.Type, sentinels []frontend.Type) (*unionInfo, error) {
	gt, err := r.renderObject(obj)
	if err != nil {
		return nil, err
	}
	// renderObject returns *Struct; name the object arm after the struct so the tag and
	// constructor read as PointOrNullPoint / PointOrNullOfPoint, the discriminated
	// union's own suffix-from-shape convention, and fall back to a generic Obj if the
	// pointer shape is ever not a plain named struct.
	structName := "Obj"
	if star, ok := gt.(*ast.StarExpr); ok {
		if id, ok := star.X.(*ast.Ident); ok {
			structName = id.Name
		}
	}
	props := map[string]bool{}
	for _, p := range r.prog.Properties(obj) {
		props[p.Name] = true
	}
	arms := []unionArm{{
		primArm:   primArm{field: unexportedName(structName), suffix: structName, typeof: "object", rank: -1},
		goType:    gt,
		isObject:  true,
		memberSig: structuralKey(r.prog, obj, map[int]int{}),
		props:     props,
	}}
	for _, s := range sentinels {
		a, ok := sentinelArm(s.Flags)
		if !ok {
			return nil, &NotYetLowerable{Flags: t.Flags, Reason: "union mixing object and non-object members is a later slice"}
		}
		arms = append(arms, unionArm{primArm: a})
	}
	sortUnionArmsByRank(arms)
	suffixes := make([]string, len(arms))
	for i, a := range arms {
		suffixes[i] = a.suffix
	}
	goName := r.decls.reserveName(strings.Join(suffixes, "Or"))
	info := &unionInfo{goName: goName, tagType: goName + "Tag", arms: arms}
	r.unions = append(r.unions, info)
	r.unionBySig[sig] = info
	return info, nil
}

// sortUnionArmsByRank orders interned arms by their fixed rank so the tag assignment
// and the generated name are independent of the order the checker lists the union
// members, the same order-independence sortArmsByRank keeps for the primitive arms. The
// object arm carries rank -1 so it sorts ahead of the sentinels.
func sortUnionArmsByRank(arms []unionArm) {
	for i := 1; i < len(arms); i++ {
		for j := i; j > 0 && arms[j-1].rank > arms[j].rank; j-- {
			arms[j-1], arms[j] = arms[j], arms[j-1]
		}
	}
}

// discriminant finds the property shared by every member of an object union whose
// value is a distinct string literal in each, the property a narrowing switch or
// compare tests. It scans the first member's properties in a stable order and picks
// the first that is a string literal in every member with no repeated value, so the
// choice does not depend on checker property order. It returns the property name and
// the per-member literal values in member order, or false when no property qualifies.
func (r *Renderer) discriminant(members []frontend.Type) (string, []string, bool) {
	names := make([]string, 0)
	for _, p := range r.prog.Properties(members[0]) {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	for _, name := range names {
		values := make([]string, len(members))
		seen := map[string]bool{}
		ok := true
		for i, m := range members {
			v, found := r.stringLiteralProp(m, name)
			if !found || seen[v] {
				ok = false
				break
			}
			seen[v] = true
			values[i] = v
		}
		if ok {
			return name, values, true
		}
	}
	return "", nil, false
}

// stringLiteralProp returns the string-literal value of a named property on an
// object type, the discriminant test's per-member value. It returns false when the
// object has no such property or the property is not a string literal.
func (r *Renderer) stringLiteralProp(t frontend.Type, name string) (string, bool) {
	for _, p := range r.prog.Properties(t) {
		if p.Name != name {
			continue
		}
		lit, ok := r.prog.LiteralValue(p.Type)
		if !ok || lit.Kind != frontend.LiteralString {
			return "", false
		}
		return lit.Str, true
	}
	return "", false
}

// unexportedName lowercases the first rune of an exported name so an object arm's
// struct field (circle) reads as an unexported field beside the primitive arms,
// while its tag and constructor keep the exported suffix (Circle).
func unexportedName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToLower(name[:1]) + name[1:]
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
	if t.Flags&frontend.TypeUnion == 0 {
		return nil, false
	}
	info, err := r.internUnion(t)
	if err != nil {
		return nil, false
	}
	return info, true
}

// unionStringValued reports whether an operand being coerced to a string is a
// tagged-sum union whose string form the ToString method can spell, and marks the
// union so the renderer emits that method. It fires for a primitive union all of
// whose arms are a number, string, boolean, or a tag-only sentinel; a union carrying
// a bigint or object arm has no spelled string form here and reports false, keeping
// the caller on its handback. An operand whose type is not an interned union reports
// false too, so an ordinary string, number, or dynamic value keeps its own coercion.
func (r *Renderer) unionStringValued(n frontend.Node) (*unionInfo, bool) {
	info, ok := r.unionInfoOrIntern(r.prog.TypeAt(n))
	if !ok || !unionToStringSupported(info) {
		return nil, false
	}
	info.needsToString = true
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
	// A union local the checker narrowed to one arm at this use (a const the flow
	// analysis pinned to its initializer's member, then passed on as the union) still
	// holds the whole union value, so it flows on as the bare local rather than
	// unwrapping the narrowed arm and rebuilding the same tag. This keeps the pass
	// one struct copy instead of a field read and a reconstruction, and reads as the
	// plain variable it is.
	if src.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(src)); ok && r.unionLocals[name] == info {
			return ident(name), true, nil
		}
	}
	if arm, ok := info.armForFlags(r.primitiveFlags(src)); ok {
		if arm.tagOnly {
			// A sentinel value flowing into the union sets the tag alone through the
			// no-argument constructor, so the lowered source expr (value.Undefined or
			// value.Null) is dropped. That is sound only when reading the source has no
			// side effect, the case for the bare undefined and null literals that reach
			// here; a side-effecting source whose type is the sentinel hands back rather
			// than lose its effect.
			if !r.repeatableOperand(src) {
				return nil, false, &NotYetLowerable{Reason: "constructing a union sentinel arm from a side-effecting source is a later slice"}
			}
			return &ast.CallExpr{Fun: ident(info.ctorName(arm))}, true, nil
		}
		return &ast.CallExpr{Fun: ident(info.ctorName(arm)), Args: []ast.Expr{expr}}, true, nil
	}
	// An object value flowing into an object union: its structural key names the arm,
	// and expr is already the pointer the object literal lowered to (&Struct{...}), so
	// it drops straight into the arm's pointer-taking constructor with no extra
	// address-of. A source whose shape matches no arm hands back rather than guess.
	st := r.prog.TypeAt(src)
	if st.Flags&frontend.TypeObject != 0 && st.Flags&frontend.TypeUnion == 0 {
		if arm, ok := info.armByMemberSig(structuralKey(r.prog, st, map[int]int{})); ok {
			return &ast.CallExpr{Fun: ident(info.ctorName(arm)), Args: []ast.Expr{expr}}, true, nil
		}
	}
	return nil, false, &NotYetLowerable{Reason: "constructing this union from its source type is a later slice"}
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
	if arm, ok := info.armForFlags(r.primitiveFlags(n)); ok {
		if arm.tagOnly {
			// The reference narrowed to a sentinel: there is no field to read, so it
			// reads as the one value that arm holds, the value.Undefined or value.Null
			// singleton the bare literal lowers to.
			r.requireImport(valuePkg)
			return sel("value", arm.singletonName()), true
		}
		return &ast.SelectorExpr{X: ident(name), Sel: ident(arm.field)}, true
	}
	// An object union narrows to a single member: the checker types the reference
	// as one object, no longer the whole union, so its structural key names the arm
	// and the read selects that arm's pointer field (name.circle), which a following
	// member access dots into (name.circle.R). A reference still typed as the whole
	// union carries the union bit and matches no object arm, so the bare struct
	// passes through.
	nt := r.prog.TypeAt(n)
	if nt.Flags&frontend.TypeUnion != 0 || nt.Flags&frontend.TypeObject == 0 {
		return nil, false
	}
	if arm, ok := info.armByMemberSig(structuralKey(r.prog, nt, map[int]int{})); ok {
		return &ast.SelectorExpr{X: ident(name), Sel: ident(arm.field)}, true
	}
	return nil, false
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
	matches := 0
	for _, a := range info.arms {
		if a.typeof == lit {
			arm = a
			matches++
		}
	}
	// A typeof string that names no arm does not narrow. One that names more than one
	// arm does not either: an object arm and a null sentinel both report "object", so
	// typeof x === "object" cannot tell them apart and must not fold to a single tag
	// test. Both leave the compare to the caller, which hands the typeof back.
	if matches != 1 {
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

// unionSentinelCompare lowers an equality between a tagged-sum union local and the
// bare undefined or null keyword to a tag compare, the sentinel-narrowing companion
// of typeofUnionCompare: x === undefined on a number | string | undefined lowers to
// comparing the tag against the undefined arm rather than building the sentinel and
// matching it, and x === null likewise. One operand must be a union local and the
// other exactly the undefined or null keyword. It returns false for any other shape,
// so a union with no matching sentinel arm (the compare is a constant TypeScript does
// not narrow on) falls through to the caller, and negates the result for !==.
func (r *Renderer) unionSentinelCompare(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	if opText != "===" && opText != "!==" {
		return nil, false, nil
	}
	name, info, flag, ok := r.unionAndSentinel(left, right)
	if !ok {
		return nil, false, nil
	}
	arm, ok := info.armForFlags(flag)
	if !ok {
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

// unionAndSentinel picks the union local and the sentinel keyword out of a
// comparison's two sides, in either order. It returns the local name, its interned
// descriptor, and the sentinel's flag (TypeUndefined or TypeNull) when exactly one
// side is a tagged-sum union local and the other is exactly the undefined or null
// keyword, and false otherwise.
func (r *Renderer) unionAndSentinel(a, b frontend.Node) (string, *unionInfo, frontend.TypeFlags, bool) {
	if name, info, ok := r.unionLocalRef(a); ok {
		if flag, ok := r.sentinelFlag(b); ok {
			return name, info, flag, true
		}
	}
	if name, info, ok := r.unionLocalRef(b); ok {
		if flag, ok := r.sentinelFlag(a); ok {
			return name, info, flag, true
		}
	}
	return "", nil, 0, false
}

// unionLocalRef reports whether a node is a bare reference to a tagged-sum union
// local, returning the local name and its descriptor. It is the receiver side of a
// sentinel compare, the union whose tag the compare tests.
func (r *Renderer) unionLocalRef(n frontend.Node) (string, *unionInfo, bool) {
	if n.Kind() != frontend.NodeIdentifier {
		return "", nil, false
	}
	name, ok := localName(r.prog.Text(n))
	if !ok {
		return "", nil, false
	}
	info, ok := r.unionLocals[name]
	if !ok {
		return "", nil, false
	}
	return name, info, true
}

// sentinelFlag reports whether a node is exactly the undefined or null keyword,
// returning TypeUndefined or TypeNull. A node whose type merely includes the sentinel
// facet (a wider type) is not the bare keyword and returns false.
func (r *Renderer) sentinelFlag(n frontend.Node) (frontend.TypeFlags, bool) {
	switch r.prog.TypeAt(n).Flags {
	case frontend.TypeUndefined:
		return frontend.TypeUndefined, true
	case frontend.TypeNull:
		return frontend.TypeNull, true
	}
	return 0, false
}

// discriminantUnionCompare lowers a discriminant test on an object union against a
// string literal to a tag compare, the property-narrowing of section 9: s.kind ===
// "circle" on a shape | circle union lowers to comparing s.tag rather than reading a
// kind field and matching a string. One side must be a read of the union's
// discriminant property off a union local, the other a string literal naming one of
// the arms. It returns false for any other shape so the caller falls through to the
// value compare, and negates the result for !==.
func (r *Renderer) discriminantUnionCompare(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	if opText != "===" && opText != "!==" {
		return nil, false, nil
	}
	name, info, lit, ok := r.discOperandAndLiteral(left, right)
	if !ok {
		return nil, false, nil
	}
	arm, ok := info.armByDisc(lit)
	if !ok {
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

// discOperandAndLiteral picks the discriminant read and the string literal out of a
// comparison's two sides, in either order. It returns the union local name, its
// interned descriptor, and the literal value when exactly one side reads the union's
// discriminant property off a union local and the other is a string literal, and
// false otherwise.
func (r *Renderer) discOperandAndLiteral(a, b frontend.Node) (string, *unionInfo, string, bool) {
	if name, info, ok := r.discriminantRead(a); ok {
		if lit, ok := r.stringLiteralValue(b); ok {
			return name, info, lit, true
		}
	}
	if name, info, ok := r.discriminantRead(b); ok {
		if lit, ok := r.stringLiteralValue(a); ok {
			return name, info, lit, true
		}
	}
	return "", nil, "", false
}

// discriminantRead reports whether a node reads the discriminant property of an
// object union off a union local, s.kind where s is a local of an object union and
// kind is its discriminant. It returns the local name and the union descriptor so
// the compare can emit s.tag against the arm the literal names.
func (r *Renderer) discriminantRead(n frontend.Node) (string, *unionInfo, bool) {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return "", nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
		return "", nil, false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok {
		return "", nil, false
	}
	info, ok := r.unionLocals[name]
	if !ok || info.disc == "" || r.prog.Text(kids[1]) != info.disc {
		return "", nil, false
	}
	return name, info, true
}

// inUnionCompare lowers a property-presence test on an object union, "r" in s, to a
// tag test, the in-narrowing of section 9: TypeScript narrows s to the arms that
// carry the named property, so the test lowers to comparing the tag against exactly
// those arms rather than probing a runtime property map. The left operand must be a
// string literal and the right a union local; the arms are split into the ones that
// carry the property and the ones that do not. When every arm carries it or none
// does the test does not narrow, so it falls through to the caller. Otherwise it
// emits the disjunction s.tag == A || s.tag == B over the arms that carry it, a
// chain of integer compares with no map lookup.
func (r *Renderer) inUnionCompare(left, right frontend.Node) (ast.Expr, bool, error) {
	prop, ok := r.stringLiteralValue(left)
	if !ok || right.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(right))
	if !ok {
		return nil, false, nil
	}
	info, ok := r.unionLocals[name]
	if !ok || info.disc == "" {
		return nil, false, nil
	}
	var have []unionArm
	for _, a := range info.arms {
		if a.isObject && a.props[prop] {
			have = append(have, a)
		}
	}
	// A property on every arm or on none does not tell the arms apart, so the test is
	// a constant TypeScript does not narrow on; leave it to the caller rather than emit
	// an always-true or always-false tag chain.
	if len(have) == 0 || len(have) == len(info.arms) {
		return nil, false, nil
	}
	var expr ast.Expr
	for _, a := range have {
		cmp := &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ident(name), Sel: ident("tag")},
			Op: token.EQL,
			Y:  ident(info.tagConst(a)),
		}
		if expr == nil {
			expr = cmp
		} else {
			expr = &ast.BinaryExpr{X: expr, Op: token.LOR, Y: cmp}
		}
	}
	return expr, true, nil
}

// inReceiver lowers the right operand of a general `key in obj` to an object value the
// runtime existence check can read, reporting whether it produced one. A dynamic value
// is already a box and lowers as itself. An object or array literal boxes member by
// member into a live value.Object, so `"a" in {a: 1}` lowers even though the literal's
// own type is a fixed shape. A static fixed-shape object binding has no box yet, so it
// returns false and the caller hands the whole `in` back.
func (r *Renderer) inReceiver(right frontend.Node) (ast.Expr, bool, error) {
	if r.isDynamic(right) {
		e, err := r.lowerExpr(right)
		if err != nil {
			return nil, false, err
		}
		return e, true, nil
	}
	if boxed, ok, err := r.boxLiteralToDynamic(right); err != nil {
		return nil, false, err
	} else if ok {
		return boxed, true, nil
	}
	return nil, false, nil
}

// objectPrototypeMembers is the set of string keys Object.prototype carries, the
// members a `key in obj` check finds on the prototype chain of any ordinary object no
// matter what its own shape declares. A static object shape written in TypeScript is an
// ordinary object whose chain ends at Object.prototype, so `"toString" in obj` and its
// siblings are always present regardless of the own fields, which is what lets the fold
// answer them a constant true. The list is the enumerable-in-membership own keys of
// Object.prototype: the four value methods, the four reflection methods, constructor,
// and the legacy __proto__ accessor and its define/lookup helpers.
var objectPrototypeMembers = map[string]bool{
	"constructor":          true,
	"hasOwnProperty":       true,
	"isPrototypeOf":        true,
	"propertyIsEnumerable": true,
	"toLocaleString":       true,
	"toString":             true,
	"valueOf":              true,
	"__proto__":            true,
	"__defineGetter__":     true,
	"__defineSetter__":     true,
	"__lookupGetter__":     true,
	"__lookupSetter__":     true,
}

// inStaticShapeRequired folds "key" in obj to the constant true when the membership is
// provably present on a static object shape, the value the boxing InOperator would
// answer at run time without a box to build. Two cases fold. A required own property
// named key is always present. A key that names an Object.prototype member is present on
// every ordinary object's prototype chain, so it holds even when the shape declares no
// such field, or declares one only optionally: the prototype member stands in when the
// own field is absent. Every other case returns not-handled so the caller keeps its
// honest handback, since none of them is a provable present. An optional member the
// shape declares under a non-prototype name may be absent; a member the shape does not
// declare may still live on Object.prototype under a name not in the set, so a false
// fold would be unsound; a non-literal key is not known here; and a receiver that is not
// side-effect-free would lose its effect once the receiver is dropped. The receiver is
// never dynamic at this point, since inReceiver has already taken the boxed path for a
// dynamic value, so reading its declared properties is safe.
func (r *Renderer) inStaticShapeRequired(left, right frontend.Node) (ast.Expr, bool) {
	prop, ok := r.stringLiteralValue(left)
	if !ok {
		return nil, false
	}
	if !r.repeatableOperand(right) {
		return nil, false
	}
	// An Object.prototype member is present on the chain of any ordinary object shape,
	// so it folds true ahead of the own-property scan, which lets a name like toString
	// hold even where the shape declares it optionally or not at all.
	if objectPrototypeMembers[prop] {
		return ident("true"), true
	}
	for _, p := range r.prog.Properties(r.prog.TypeAt(right)) {
		if p.Name != prop {
			continue
		}
		if p.Optional {
			return nil, false
		}
		return ident("true"), true
	}
	return nil, false
}

// elidedInReceiver reports the identifier receiver a required-member in fold drops from
// the emit. "key" in obj folds to the constant true when obj carries a required own
// property named key and never lowers obj, so a binding whose only read was that
// receiver would be declared and not used in Go. Recording the read lets bindingUnused
// blank it. The match is the one inStaticShapeRequired makes, restricted to a bare
// identifier receiver since only a binding can be orphaned: a literal or property-access
// receiver names no local to blank. An over-count is harmless, since a receiver that
// does not fold hands the whole unit back and emits no Go.
func elidedInReceiver(r *Renderer, n frontend.Node) (frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 3 || r.prog.Text(kids[1]) != "in" {
		return nil, false
	}
	left, right := kids[0], kids[2]
	if right.Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	if _, ok := r.inStaticShapeRequired(left, right); !ok {
		return nil, false
	}
	return right, true
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
		out = append(out, unionJSONArm(info))
		if info.needsTypeOf {
			out = append(out, unionTypeOf(info))
		}
		if info.needsToBoolean {
			out = append(out, unionToBoolean(info))
		}
		if info.needsToString {
			out = append(out, unionToString(info))
		}
	}
	return out
}

// unionTypeOf builds the TypeOf method a bare typeof over a primitive union lowers
// to. The value struct carries no self-describing box, so typeof cannot ask it for
// its kind the way a dynamic value.Value answers; instead the method switches on the
// tag and returns the arm's typeof string, the tag each arm already pins down at
// construction. It returns a value.BStr, the same string type the folded typeof of a
// known type and the dynamic value.Value.TypeOf both yield, so the result flows on as
// a plain string. The trailing return is unreachable, the tag always matching an arm,
// and only satisfies Go's terminating-statement rule.
func unionTypeOf(info *unionInfo) ast.Decl {
	cases := make([]ast.Stmt, 0, len(info.arms))
	for _, a := range info.arms {
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{ident(info.tagConst(a))},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				&ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(a.typeof)}}},
			}}},
		})
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("u")}, Type: ident(info.goName)}}},
		Name: ident("TypeOf"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "BStr")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{
				Tag:  &ast.SelectorExpr{X: ident("u"), Sel: ident("tag")},
				Body: &ast.BlockStmt{List: cases},
			},
			&ast.ReturnStmt{Results: []ast.Expr{
				&ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("undefined")}}},
			}},
		}},
	}
}

// unionToBooleanSupported reports whether every arm of a union has a truthiness
// unionToBoolean can spell: a value arm that is a number, string, or boolean, whose
// inline ToBoolean truthyOfKind builds, or a tag-only sentinel (undefined, null),
// which is always falsy. A bigint or object arm has no inline truthiness here yet, so
// a union carrying one is not supported and its truthiness keeps the handback.
func unionToBooleanSupported(info *unionInfo) bool {
	for _, a := range info.arms {
		if a.tagOnly {
			continue
		}
		if _, ok := primitiveTruthyKind(a.flag); !ok {
			return false
		}
	}
	return true
}

// unionToBoolean builds the ToBoolean method a value of this union lowers to when it
// stands in boolean position. JavaScript reads any value there through its falsy set,
// which for a union means the falsy rule of the arm the tag selects: a number is
// falsy at zero and NaN, a string when empty, a boolean is its own truth, and the
// undefined and null sentinels are always falsy. The method switches on the tag and
// returns each value arm's inline ToBoolean; the tag-only sentinels emit no case and
// fall to the trailing return false, the truth every sentinel has. It returns a plain
// Go bool, the value the if, for, or ! position wants, and evaluating the union once
// through the method call keeps a side-effecting operand's effect that the inlined
// two-name form could not.
func unionToBoolean(info *unionInfo) ast.Decl {
	cases := make([]ast.Stmt, 0, len(info.arms))
	for _, a := range info.arms {
		if a.tagOnly {
			continue
		}
		kind, _ := primitiveTruthyKind(a.flag)
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{ident(info.tagConst(a))},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				truthyOfKind(&ast.SelectorExpr{X: ident("u"), Sel: ident(a.field)}, kind),
			}}},
		})
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("u")}, Type: ident(info.goName)}}},
		Name: ident("ToBoolean"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ident("bool")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{
				Tag:  &ast.SelectorExpr{X: ident("u"), Sel: ident("tag")},
				Body: &ast.BlockStmt{List: cases},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ident("false")}},
		}},
	}
}

// unionToStringSupported reports whether every arm of a union has a string form
// unionToString can spell: a value arm that is a number, string, or boolean, whose
// ToString is value.NumberToString, the string itself, or value.BoolToString, or a
// tag-only sentinel (undefined, null), whose string is the literal "undefined" or
// "null". A bigint or object arm coerces to a string through a path this slice does
// not spell, so a union carrying one is not supported and its coercion keeps the
// handback.
func unionToStringSupported(info *unionInfo) bool {
	for _, a := range info.arms {
		if a.tagOnly {
			continue
		}
		if _, ok := armStringExpr(a); !ok {
			return false
		}
	}
	return true
}

// armStringExpr returns the value.BStr expression an arm's ToString case emits,
// reading the arm field off the receiver u: a number through value.NumberToString, a
// string as itself (it is already a value.BStr), a boolean through value.BoolToString,
// and a tag-only sentinel as the literal string JavaScript reports for it, "undefined"
// or "null". It returns ok false for a bigint or object arm this slice does not spell.
func armStringExpr(a unionArm) (ast.Expr, bool) {
	if a.tagOnly {
		lit := "undefined"
		if a.flag == frontend.TypeNull {
			lit = "null"
		}
		return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, true
	}
	field := &ast.SelectorExpr{X: ident("u"), Sel: ident(a.field)}
	switch {
	case a.flag&frontend.TypeNumber != 0:
		return &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{field}}, true
	case a.flag&frontend.TypeString != 0:
		return field, true // already a value.BStr, the identity
	case a.flag&frontend.TypeBoolean != 0:
		return &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{field}}, true
	}
	return nil, false
}

// unionToString builds the ToString method a value of this union lowers to when it is
// coerced to a string, by String(x), a template substitution, or a string
// concatenation. JavaScript reads a value there through ToString, which for a union is
// the string form of the arm the tag selects: a number through Number::toString, a
// string as itself, a boolean as "true" or "false", and the undefined and null
// sentinels as "undefined" and "null". The method switches on the tag and returns each
// arm's string; every arm carries a case, including the sentinels, since each has a
// definite string. It returns a value.BStr, the same string type the other coercions
// yield, so the result flows on as a plain string, and evaluating the union once
// through the method call keeps a side-effecting operand's effect.
func unionToString(info *unionInfo) ast.Decl {
	cases := make([]ast.Stmt, 0, len(info.arms))
	for _, a := range info.arms {
		expr, _ := armStringExpr(a)
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{ident(info.tagConst(a))},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{expr}}},
		})
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("u")}, Type: ident(info.goName)}}},
		Name: ident("ToString"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "BStr")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{
				Tag:  &ast.SelectorExpr{X: ident("u"), Sel: ident("tag")},
				Body: &ast.BlockStmt{List: cases},
			},
			&ast.ReturnStmt{Results: []ast.Expr{
				&ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `""`}}},
			}},
		}},
	}
}

// unionJSONArm builds the JSONArm method that hands the union's active member to the
// JSON serializer. A tagged-sum union stores its value in the field its tag selects,
// and those fields are unexported, so without this hook JSON.stringify would reflect
// the struct and write an empty object. The method switches on the tag and returns
// the matching arm boxed as any, the member the serializer then renders as the value
// the union holds. It is a plain method a person would write to make the value
// serializable, and the encoder recognizes it by the exported name.
func unionJSONArm(info *unionInfo) ast.Decl {
	cases := make([]ast.Stmt, 0, len(info.arms))
	for _, a := range info.arms {
		result := ast.Expr(&ast.SelectorExpr{X: ident("u"), Sel: ident(a.field)})
		if a.tagOnly {
			// A sentinel arm has no field to hand the serializer; nil renders as JSON
			// null, which is what a null arm is and the closest the encoder has for an
			// undefined the serializer would otherwise omit.
			result = ident("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{ident(info.tagConst(a))},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{result}}},
		})
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("u")}, Type: ident(info.goName)}}},
		Name: ident("JSONArm"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ident("any")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{
				Tag:  &ast.SelectorExpr{X: ident("u"), Sel: ident("tag")},
				Body: &ast.BlockStmt{List: cases},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ident("nil")}},
		}},
	}
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
		if a.tagOnly {
			// A sentinel arm stores no value, so it contributes only its tag and no field.
			continue
		}
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
	// A sentinel arm carries no payload, so its constructor takes no parameter and sets
	// the tag alone; the value arm's takes the value and sets both tag and field.
	if a.tagOnly {
		return &ast.FuncDecl{
			Name: ident(info.ctorName(a)),
			Type: &ast.FuncType{
				Params:  &ast.FieldList{},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ident(info.goName)}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				&ast.CompositeLit{
					Type: ident(info.goName),
					Elts: []ast.Expr{
						&ast.KeyValueExpr{Key: ident("tag"), Value: ident(info.tagConst(a))},
					},
				},
			}}}},
		}
	}
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
