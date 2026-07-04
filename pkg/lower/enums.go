package lower

import (
	"go/ast"
	"go/token"
	"math"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers enums (05_type_lowering, the enums section). A plain numeric
// enum becomes a float64-backed constant per member, so a member read A.B lowers
// to the constant AB and every use of an enum value rides the number path
// unchanged: the checker types an enum member as number, and typeExpr maps an
// enum-typed slot to float64, so a comparison, a switch, a binding, and a console
// print all see the float64 the value already is. A named Go type would fight that
// float64 floor and buy nothing the checker has not already enforced, so the
// representation is the bare constants.
//
// A plain string enum becomes a value.BStr package var per member: a bento string
// is a runtime value with no Go constant form, so the members are vars rather than
// constants, and an enum-typed slot lowers to value.BStr the same way the string
// case does. A member read names the var, and every use rides the string path,
// since primitiveFlags folds a string enum to string just as it folds a numeric
// enum to number.
//
// A const enum emits no type and no members: each member read inlines the member's
// value at the use site (a numeric literal for a numeric enum, a value.FromGoString
// for a string enum), the erasure TypeScript itself performs.
//
// A heterogeneous enum that mixes numeric and string members has no single Go type
// and hands back, as do computed members, routing the unit to the engine rather
// than lowering a shape this file does not build.

// enumInfo is one registered enum. A const enum keeps its members for the inline
// lookup but emits nothing at package level; a plain numeric enum emits a const
// block and each member read names the member's Go constant, while a plain string
// enum emits a var block of value.BStr initializers, since a bento string is a
// runtime value with no Go constant form.
type enumInfo struct {
	name     string // the TypeScript enum name
	goName   string // the exported Go name the member names are prefixed with
	isConst  bool
	isString bool // a string enum: members hold value.BStr strings, not float64s
	members  []enumMember
}

// enumMember is one member of an enum: its source name, the Go name a plain enum's
// read resolves to (goName + the exported member name), and its value. A numeric
// member carries a float64 in value; a string member carries its UTF-16 code units
// in strUnits, the same content a string literal decodes to.
type enumMember struct {
	name     string
	goConst  string
	value    float64
	strUnits []uint16
}

// memberByName returns the member with the given source name, so a read E.M finds
// the constant M lowers to. A miss reports false and the caller hands the read
// back rather than inventing a constant.
func (e *enumInfo) memberByName(name string) (enumMember, bool) {
	for _, m := range e.members {
		if m.name == name {
			return m, true
		}
	}
	return enumMember{}, false
}

// collectEnums registers every top-level numeric enum in the entry module before
// any body lowers, so a member read that appears above the enum's declaration
// still resolves. It mirrors collectClasses: it gathers the enum declarations,
// builds the set of identifiers the module already speaks, and registers each
// enum against that set so a minted constant name that would collide with an
// existing name hands back instead of shadowing.
func (r *Renderer) collectEnums(entry frontend.Node) error {
	var enumDecls []frontend.Node
	for _, stmt := range r.prog.Children(entry) {
		if stmt.Kind() == frontend.NodeEnumDeclaration {
			enumDecls = append(enumDecls, stmt)
		}
	}
	if len(enumDecls) == 0 {
		return nil
	}
	taken := map[string]bool{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeIdentifier {
			taken[r.prog.Text(n)] = true
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
	for _, decl := range enumDecls {
		if err := r.registerEnum(decl, taken); err != nil {
			return err
		}
	}
	return nil
}

// registerEnum validates one enum declaration against the numeric subset and
// records it. The leading unnamed nodes carry the modifiers (const, export), read
// the same structural way a class reads abstract; the name follows, then one
// unnamed member node per member. A member's value is its numeric-literal
// initializer, or one past the previous member when it has none, the auto-increment
// TypeScript assigns. A string-valued member, a computed initializer, or a
// duplicate constant name hands the whole enum back.
func (r *Renderer) registerEnum(decl frontend.Node, taken map[string]bool) error {
	kids := r.prog.Children(decl)
	isConst := false
	// Leading unnamed nodes are modifiers, one per keyword, before the name.
	for len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown {
		if strings.TrimSpace(r.prog.Text(kids[0])) == "const" {
			isConst = true
		}
		kids = kids[1:]
	}
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "an enum without a name is a later slice"}
	}
	name := r.prog.Text(kids[0])
	goName, ok := exportedField(name)
	if !ok {
		return &NotYetLowerable{Reason: "enum name is not a Go identifier"}
	}
	if _, dup := r.enums[name]; dup {
		return &NotYetLowerable{Reason: "two enums named " + name + " in one module is a later slice"}
	}

	isString, err := r.enumIsString(name, kids[1:])
	if err != nil {
		return err
	}

	info := &enumInfo{name: name, goName: goName, isConst: isConst, isString: isString}
	next := 0.0
	minted := map[string]bool{}
	for _, memberNode := range kids[1:] {
		mkids := r.prog.Children(memberNode)
		if len(mkids) == 0 || mkids[0].Kind() != frontend.NodeIdentifier {
			return &NotYetLowerable{Reason: "enum " + name + " has a member this slice does not read"}
		}
		mName := r.prog.Text(mkids[0])

		goConst := goName + exportedOrEmpty(mName)
		if goConst == goName {
			return &NotYetLowerable{Reason: "enum " + name + " member " + mName + " is not a Go identifier"}
		}
		if minted[goConst] {
			return &NotYetLowerable{Reason: "enum " + name + " members collide on the Go name " + goConst + ", a later slice"}
		}
		// A plain enum names package-level members; a const enum inlines and speaks
		// no names, so only a plain enum checks the name against the module's existing
		// identifiers.
		if !isConst && taken[goConst] {
			return &NotYetLowerable{Reason: "the module already speaks " + goConst + ", the name enum " + name + "." + mName + " needs"}
		}
		minted[goConst] = true

		member := enumMember{name: mName, goConst: goConst}
		if isString {
			// A string enum requires every member initialized with a string literal;
			// there is no auto-increment for strings, so a bare or numeric member here is
			// the heterogeneous shape enumIsString already rejected, and this reads the
			// member's UTF-16 content the same way a string literal decodes it.
			units, ok := r.enumStringInit(mkids)
			if !ok {
				return &NotYetLowerable{Reason: "string enum " + name + "." + mName + " has an initializer that is not a string literal, a later slice"}
			}
			member.strUnits = units
		} else {
			var val float64
			if len(mkids) >= 2 {
				// An initialized member: a numeric literal, or a unary plus or minus over
				// one (the common negative-sentinel form), is folded to its value. Any other
				// computed expression is a later slice, so the whole enum hands back.
				v, ok := r.enumInitValue(mkids[len(mkids)-1])
				if !ok {
					return &NotYetLowerable{Reason: "enum " + name + "." + mName + " has an initializer that is not a numeric literal, a later slice"}
				}
				val = v
			} else {
				val = next
			}
			next = val + 1
			member.value = val
		}
		info.members = append(info.members, member)
	}

	r.enums[name] = info
	r.enumOrder = append(r.enumOrder, name)
	return nil
}

// enumIsString decides whether an enum is a string enum from its members'
// initializers. A string enum has every member initialized with a string literal;
// a numeric enum has numeric or auto-incremented members. A mix of the two is a
// number|string union with no single Go type, so it hands back rather than lower to
// one representation that would lose the other half.
func (r *Renderer) enumIsString(name string, memberNodes []frontend.Node) (bool, error) {
	hasString, hasNumeric := false, false
	for _, memberNode := range memberNodes {
		mkids := r.prog.Children(memberNode)
		if len(mkids) >= 2 && mkids[len(mkids)-1].Kind() == frontend.NodeStringLiteral {
			hasString = true
		} else {
			hasNumeric = true
		}
	}
	if hasString && hasNumeric {
		return false, &NotYetLowerable{Reason: "a heterogeneous enum mixing number and string members like " + name + " is a later slice"}
	}
	return hasString, nil
}

// enumStringInit reads a string-enum member's UTF-16 content from its string-literal
// initializer, decoding the source escapes the way stringLiteral does so the member
// holds the same code units a string literal of the same text would. It reports false
// for a member with no string-literal initializer, so the caller hands the enum back.
func (r *Renderer) enumStringInit(mkids []frontend.Node) ([]uint16, bool) {
	if len(mkids) < 2 {
		return nil, false
	}
	init := mkids[len(mkids)-1]
	if init.Kind() != frontend.NodeStringLiteral {
		return nil, false
	}
	text := r.prog.Text(init)
	if len(text) < 2 {
		return nil, false
	}
	quote := text[0]
	if (quote != '"' && quote != '\'') || text[len(text)-1] != quote {
		return nil, false
	}
	return decodeJSString(text[1 : len(text)-1])
}

// enumInitValue folds an enum member's initializer to its float64 value. It reads
// a bare numeric literal, or a unary plus or minus applied to one, which covers
// the explicit and negative-sentinel members an enum commonly writes. Anything
// else (a string literal, a reference to another member, a computed expression)
// reports false so the whole enum hands back to a later slice.
func (r *Renderer) enumInitValue(init frontend.Node) (float64, bool) {
	if init.Kind() == frontend.NodeNumericLiteral {
		return numericLiteralValue(r.prog.Text(init))
	}
	if init.Kind() == frontend.NodePrefixUnaryExpression {
		kids := r.prog.Children(init)
		if len(kids) != 1 {
			return 0, false
		}
		operand := kids[0]
		op := strings.TrimSpace(strings.TrimSuffix(r.prog.Text(init), r.prog.Text(operand)))
		v, ok := r.enumInitValue(operand)
		if !ok {
			return 0, false
		}
		switch op {
		case "+":
			return v, true
		case "-":
			return -v, true
		default:
			return 0, false
		}
	}
	return 0, false
}

// exportedOrEmpty is the exported Go spelling of a member name, or "" when the
// name is not a legal Go identifier, so registerEnum can flag the member rather
// than mint an unsound constant.
func exportedOrEmpty(name string) string {
	e, ok := exportedField(name)
	if !ok {
		return ""
	}
	return e
}

// enumOfType returns the registered enum this type belongs to, matched by the
// type's symbol name. The whole enum type an annotation names (a parameter,
// return, or typed binding) carries the enum's symbol beside a union of its
// members, so this resolves it the same way a member read resolves through the
// name map. An enum this file declined to register (a heterogeneous or computed
// one) reports false.
func (r *Renderer) enumOfType(t frontend.Type) (*enumInfo, bool) {
	if t.Flags&frontend.TypeEnum == 0 {
		return nil, false
	}
	sym, ok := r.prog.TypeSymbol(t)
	if !ok {
		return nil, false
	}
	info, ok := r.enums[sym.Name]
	return info, ok
}

// enumMemberRead lowers a member read E.M when E is a registered enum. A plain
// enum resolves to the member's Go constant; a const enum inlines the member's
// numeric value as a literal, the same shape a numeric literal takes so the
// binding and coercion paths float-ify it exactly as they would any number. It
// reports ok=false when the object is not an enum or the member is unknown, so the
// caller falls through to its other property paths.
func (r *Renderer) enumMemberRead(obj frontend.Node, prop string) (ast.Expr, bool, error) {
	if obj.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	info, ok := r.enums[r.prog.Text(obj)]
	if !ok {
		return nil, false, nil
	}
	member, ok := info.memberByName(prop)
	if !ok {
		return nil, false, &NotYetLowerable{Reason: "enum " + info.name + " has no member " + prop}
	}
	if info.isConst {
		// A const enum inlines the member's value at the use site: the string content
		// as a value.BStr, or the numeric value as a literal the number path floatifies.
		if info.isString {
			return r.bstrLit(member.strUnits), true, nil
		}
		return enumValueLit(member.value), true, nil
	}
	return ident(member.goConst), true, nil
}

// renderEnums emits the package-level block for each plain enum in source order. A
// const enum emits nothing: its members were inlined at every use site. A numeric
// enum reads like a hand-written Go enum, a float64-typed constant per member; a
// string enum is a var block of value.BStr initializers, since a bento string has
// no Go constant form:
//
//	const (
//		ColorRed   float64 = 0
//		ColorGreen float64 = 5
//		ColorBlue  float64 = 6
//	)
//
//	var (
//		DirNorth = value.FromGoString("NORTH")
//		DirSouth = value.FromGoString("SOUTH")
//	)
func (r *Renderer) renderEnums() []ast.Decl {
	var out []ast.Decl
	for _, name := range r.enumOrder {
		info := r.enums[name]
		if info.isConst || len(info.members) == 0 {
			continue
		}
		if info.isString {
			specs := make([]ast.Spec, 0, len(info.members))
			for _, m := range info.members {
				specs = append(specs, &ast.ValueSpec{
					Names:  []*ast.Ident{ident(m.goConst)},
					Values: []ast.Expr{r.bstrLit(m.strUnits)},
				})
			}
			out = append(out, &ast.GenDecl{Tok: token.VAR, Lparen: token.Pos(1), Specs: specs})
			continue
		}
		specs := make([]ast.Spec, 0, len(info.members))
		for _, m := range info.members {
			specs = append(specs, &ast.ValueSpec{
				Names:  []*ast.Ident{ident(m.goConst)},
				Type:   ident("float64"),
				Values: []ast.Expr{enumValueLit(m.value)},
			})
		}
		out = append(out, &ast.GenDecl{Tok: token.CONST, Lparen: token.Pos(1), Specs: specs})
	}
	return out
}

// enumValueLit renders an enum member's float64 value as a Go literal. A whole
// number prints without a fraction (5, not 5.0) since the constant's declared
// float64 type carries the floatness, and a fractional value prints its shortest
// exact decimal. It matches numericLiteral's spelling so an inlined const-enum
// value rides the same float-ification the binding and coercion paths apply to any
// number.
func enumValueLit(v float64) ast.Expr {
	if math.Signbit(v) {
		// A Go basic literal is unsigned; a negative value is a unary minus over the
		// positive literal, the shape go/printer expects.
		return &ast.UnaryExpr{Op: token.SUB, X: enumValueLit(-v)}
	}
	if v == math.Trunc(v) && v <= 1e15 {
		return &ast.BasicLit{Kind: token.INT, Value: strconv.FormatFloat(v, 'f', -1, 64)}
	}
	return &ast.BasicLit{Kind: token.FLOAT, Value: strconv.FormatFloat(v, 'f', -1, 64)}
}
