package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers classes (05_type_lowering sections on classes and method
// sets). A class whose methods are only ever called on statically typed
// instances needs no dynamic dispatch, so the core slice is the direct mapping a
// Go programmer writes by hand: the class becomes a struct, the constructor
// becomes a NewX function returning *X, each method becomes a pointer-receiver
// method, and this becomes the receiver. Statics belong to the class, not the
// instance, so a static field becomes a package-level var and a static method a
// package-level function, both named after the class the way a Go package
// spells them (aTotal, ABump). A get accessor becomes a plain method and a set
// accessor its Set-prefixed sibling, the Go accessor idiom, with the property
// read and write sites rewritten to the calls. Everything that needs more
// machinery (inheritance and super, a method used as a value, virtual dispatch
// through a base-typed variable) hands back to the engine, the section 30
// boundary every other lowering keeps.
//
// The registry of top-level classes is collected in a pre-pass before any body
// lowers, so a function body can construct an instance of a class declared
// below it, the same hoisting the checker applies. An instance is recognized at
// a use site through the checker, not by shape: the instance type's declaring
// symbol (frontend.TypeSymbol) is walked back to the registered declaration, so
// a structural twin of a class shape is never mistaken for the class and a
// class whose fields happen to spell a Map or Uint8Array fingerprint is never
// hijacked by those paths.

// classInfo is one registered top-level class.
type classInfo struct {
	name   string // source class name, the registry key
	goName string // Go type name, exportedField of name
	recv   string // receiver identifier shared by the constructor and every method
	decl   frontend.Node
	fields []classField
	ctor   frontend.Node // nil when the class declares no constructor
	// ctorParams are the constructor's parameter nodes, kept so a new expression
	// coerces each argument against the declared parameter, and empty for the
	// default constructor.
	ctorParams []frontend.Node
	methods    []classMethod
	// statics are the static fields, each becoming a package-level var whose
	// goName is the var's package-unique name, and staticMethods the static
	// methods, each becoming a package-level function named goName.
	statics       []classField
	staticMethods []classMethod
	// getters and setters are the instance accessors; a getter emits as a
	// plain method and a setter as its Set-prefixed sibling, the Go accessor
	// idiom, and the property read and write sites route to the calls.
	getters []classMethod
	setters []classSetter
	// staticGetters and staticSetters are the static accessors; a static getter
	// emits as a package function named goName (CX) and a static setter as its
	// Set-prefixed sibling (CSetX), and a read or write through the class name
	// routes to the call, the static twin of the instance accessor path.
	staticGetters []classMethod
	staticSetters []classSetter
	// staticInit is the class's ordered static initialization: static blocks and
	// the assignments non-constant static field initializers become, interleaved
	// in member order. Each step lowers into a package function the program
	// assembler calls at the class declaration's position in the main body, which
	// is when JavaScript runs it; package-level Go has no ordered statement
	// execution, so the ordered work these steps do lives in that called function
	// instead. A constant static field keeps its package var initializer and adds
	// no step, so the common case emits no init function.
	staticInit []staticInitStep
	// base is the registered class this one extends, nil for a root class. The
	// base embeds as the derived struct's first field, so Go promotion serves
	// the inherited fields and methods; registration rejects a derived member
	// sharing a base member's name, so promotion never has to break a tie.
	base *classInfo
	// superArgs are the argument nodes of the constructor's super(...) call,
	// validated to sit at the point past any this-free leading statements. They
	// stay nil when the class has no constructor of its own; the synthesized
	// constructor passes its parameters (the base's) straight through.
	superArgs []frontend.Node
	// preSuper are the constructor's statements before super(), each validated to
	// be this-free and to declare no binding a later statement could read, so they
	// lower before the base assignment the way JavaScript runs them (this is in the
	// temporal dead zone until super returns).
	preSuper []frontend.Node
	// vprops, set only on a hierarchy root, names the methods some subclass
	// overrides; each becomes a slot in the root's vtable (section 13).
	// overrides names this class's own methods that fill an inherited slot.
	// extended marks a class some registered class directly extends, which is
	// what makes its constructor split out the init function a derived
	// constructor calls on the embedded base when the hierarchy is virtual.
	vprops    map[string]bool
	overrides map[string]bool
	extended  bool
	// abstract marks an abstract class: it emits no NewX (the checker rejects
	// new on it), only the init function its concrete subclasses run on the
	// embedded base, and its abstract methods hold vtable slots with no body.
	abstract bool
	// thrownAsError marks a class the module throws. Its instances travel the
	// runtime's panic path, so emission adds the ErrorName and ErrorMessage
	// methods that satisfy value.Thrown (set by lowerThrow, read by
	// renderClass).
	thrownAsError bool
}

// classField is one instance field, in declaration order.
type classField struct {
	prop   string        // source property name
	goName string        // exported Go field name, or the package var name for a static
	ident  frontend.Node // the name node, whose checker type is the declared field type
	init   frontend.Node // the initializer expression, nil when none
	// runtimeInit marks a static field whose initializer is not a constant
	// expression, so the package var declares zero-valued and the initializer runs
	// as an assignment in the class's static init function in member order rather
	// than as the var's own initializer. It is always false for an instance field.
	runtimeInit bool
}

// staticInitStep is one step of a class's ordered static initialization: either
// a static initialization block or the assignment a non-constant static field
// initializer becomes. The steps run in member order inside the class's init
// function, which is when JavaScript runs them, so a static that reads an
// earlier static (static b = C.a + 1) sees the value the earlier step wrote.
type staticInitStep struct {
	block frontend.Node // the block of a static { ... }, nil for a field step
	field *classField   // the static field whose initializer runs here, nil for a block step
}

// classMethod is one instance method. An abstract method has no body: it
// declares a vtable slot on the hierarchy root that every concrete subclass
// fills, so it emits the dispatching entry but no Impl.
type classMethod struct {
	prop      string
	goName    string
	node      frontend.Node
	abstract  bool
	generator bool
	async     bool
}

// classSetter is one set accessor; param is its single parameter's name node,
// the coercion target of the stored value.
type classSetter struct {
	prop   string
	goName string // the emitted method name, Set plus the exported property name
	node   frontend.Node
	param  frontend.Node
}

func (c *classInfo) fieldByName(prop string) (classField, bool) {
	for _, f := range c.fields {
		if f.prop == prop {
			return f, true
		}
	}
	return classField{}, false
}

func (c *classInfo) methodByName(prop string) (classMethod, bool) {
	for _, m := range c.methods {
		if m.prop == prop {
			return m, true
		}
	}
	return classMethod{}, false
}

func (c *classInfo) staticByName(prop string) (classField, bool) {
	for _, f := range c.statics {
		if f.prop == prop {
			return f, true
		}
	}
	return classField{}, false
}

func (c *classInfo) staticMethodByName(prop string) (classMethod, bool) {
	for _, m := range c.staticMethods {
		if m.prop == prop {
			return m, true
		}
	}
	return classMethod{}, false
}

func (c *classInfo) getterByName(prop string) (classMethod, bool) {
	for _, m := range c.getters {
		if m.prop == prop {
			return m, true
		}
	}
	return classMethod{}, false
}

func (c *classInfo) setterByName(prop string) (classSetter, bool) {
	for _, s := range c.setters {
		if s.prop == prop {
			return s, true
		}
	}
	return classSetter{}, false
}

func (c *classInfo) staticGetterByName(prop string) (classMethod, bool) {
	for _, g := range c.staticGetters {
		if g.prop == prop {
			return g, true
		}
	}
	return classMethod{}, false
}

func (c *classInfo) staticSetterByName(prop string) (classSetter, bool) {
	for _, s := range c.staticSetters {
		if s.prop == prop {
			return s, true
		}
	}
	return classSetter{}, false
}

// The lookup* helpers resolve an instance property against the whole base
// chain, the view a use site has of an instance: a derived instance serves its
// own members and every inherited one, and the emitted selector or call
// reaches a base's member through Go's own promotion. Registration rejects a
// derived member sharing a base member's name, so at most one class on the
// chain owns any property and the walk order cannot change the answer. The
// byName forms above stay own-only for the sites that must not chain: the
// constructor fold, which may only fold stores into fields the literal being
// built declares, and statics, which do not inherit in this slice.

func (c *classInfo) lookupField(prop string) (classField, bool) {
	for ci := c; ci != nil; ci = ci.base {
		if f, ok := ci.fieldByName(prop); ok {
			return f, true
		}
	}
	return classField{}, false
}

func (c *classInfo) lookupMethod(prop string) (classMethod, bool) {
	for ci := c; ci != nil; ci = ci.base {
		if m, ok := ci.methodByName(prop); ok {
			return m, true
		}
	}
	return classMethod{}, false
}

func (c *classInfo) lookupGetter(prop string) (classMethod, bool) {
	for ci := c; ci != nil; ci = ci.base {
		if m, ok := ci.getterByName(prop); ok {
			return m, true
		}
	}
	return classMethod{}, false
}

func (c *classInfo) lookupSetter(prop string) (classSetter, bool) {
	for ci := c; ci != nil; ci = ci.base {
		if s, ok := ci.setterByName(prop); ok {
			return s, true
		}
	}
	return classSetter{}, false
}

// collectClasses registers every top-level class of the entry module before any
// body lowers, the same pre-pass collectNodeImports runs for imports, so a use
// anywhere in the module resolves against the full registry. A class the slice
// does not cover hands the whole unit back here, before any Go is emitted.
//
// A first pass gathers every identifier the module speaks, because a class
// mints package-level names the source never wrote: the NewX constructor, a
// package var per static field, a package function per static method. A minted
// name that collides with a name the module already uses would change what an
// existing reference means, so registration checks each one against this set
// and hands back on a clash instead of shadowing.
func (r *Renderer) collectClasses(entry frontend.Node) error {
	var classDecls []frontend.Node
	for _, stmt := range r.prog.Children(entry) {
		if stmt.Kind() == frontend.NodeClassDeclaration {
			classDecls = append(classDecls, stmt)
		}
	}
	if len(classDecls) == 0 {
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
	r.classTaken = taken
	for _, decl := range classDecls {
		if err := r.registerClass(decl, taken); err != nil {
			return err
		}
	}
	return nil
}

// registerClass validates one class declaration against the covered subset and
// records it. The checks are spelling-level where the frontend does not expose
// a construct as its own node kind (a static or async modifier rides on the
// member's source text), which errs toward handing back: a modifier bento does
// not model routes the unit to the engine rather than lowering a member with
// the wrong semantics.
func (r *Renderer) registerClass(decl frontend.Node, taken map[string]bool) error {
	kids := r.prog.Children(decl)
	// The abstract modifier surfaces as an unnamed node before the name, the
	// same shape a member's static modifier takes.
	abstract := false
	if len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown && strings.TrimSpace(r.prog.Text(kids[0])) == "abstract" {
		abstract = true
		kids = kids[1:]
	}
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "a class without a name is a later slice"}
	}
	name := r.prog.Text(kids[0])
	goName, ok := exportedField(name)
	if !ok {
		return &NotYetLowerable{Reason: "class name is not a Go identifier"}
	}
	if _, dup := r.classes[name]; dup {
		return &NotYetLowerable{Reason: "two classes named " + name + " in one module is a later slice"}
	}
	// An abstract class mints no constructor name: the checker rejects new on
	// it, so only its init function ever exists at package level.
	if !abstract {
		if ctorName := "New" + goName; taken[ctorName] {
			return &NotYetLowerable{Reason: "the module already speaks " + ctorName + ", the name class " + name + "'s constructor needs"}
		} else {
			taken[ctorName] = true
		}
	}

	info := &classInfo{name: name, goName: goName, decl: decl, abstract: abstract}
	var paramProps []classField
	for _, m := range kids[1:] {
		switch m.Kind() {
		case frontend.NodeIdentifier:
			// The name, already read.
		case frontend.NodePropertyDeclaration:
			if r.memberIsStatic(m) {
				f, err := r.staticFieldOf(info, m, taken)
				if err != nil {
					return err
				}
				info.statics = append(info.statics, f)
				// A non-constant initializer runs in the static init function at its
				// member position, so it is recorded as a step here in the order it
				// appears among the blocks. The step holds a copy so a later static
				// field appended to info.statics cannot move it.
				if f.runtimeInit {
					step := f
					if err := r.noteStaticInit(info, staticInitStep{field: &step}, taken); err != nil {
						return err
					}
				}
				continue
			}
			f, err := r.classFieldOf(m)
			if err != nil {
				return err
			}
			info.fields = append(info.fields, f)
		case frontend.NodeConstructor:
			if info.ctor != nil {
				return &NotYetLowerable{Reason: "constructor overloads are a later slice"}
			}
			params, props, err := r.ctorParamsOf(m)
			if err != nil {
				return err
			}
			info.ctor = m
			info.ctorParams = params
			paramProps = props
		case frontend.NodeMethodDeclaration:
			if r.memberIsStatic(m) {
				meth, err := r.staticMethodOf(info, m, taken)
				if err != nil {
					return err
				}
				info.staticMethods = append(info.staticMethods, meth)
				continue
			}
			meth, err := r.classMethodOf(m)
			if err != nil {
				return err
			}
			if _, clash := info.fieldByName(meth.prop); clash {
				return &NotYetLowerable{Reason: "a method named like a field is a later slice"}
			}
			info.methods = append(info.methods, meth)
		case frontend.NodeGetAccessor:
			if r.memberIsStatic(m) {
				g, err := r.staticGetterOf(info, m, taken)
				if err != nil {
					return err
				}
				info.staticGetters = append(info.staticGetters, g)
				continue
			}
			g, err := r.getterOf(m)
			if err != nil {
				return err
			}
			info.getters = append(info.getters, g)
		case frontend.NodeSetAccessor:
			if r.memberIsStatic(m) {
				s, err := r.staticSetterOf(info, m, taken)
				if err != nil {
					return err
				}
				info.staticSetters = append(info.staticSetters, s)
				continue
			}
			s, err := r.setterOf(m)
			if err != nil {
				return err
			}
			info.setters = append(info.setters, s)
		case frontend.NodeUnknown:
			// A heritage clause surfaces as an unnamed node whose text starts with
			// its keyword. An extends clause names the base class this slice embeds;
			// implements stays a later slice. An empty leftover token and a stray
			// semicolon are both no-op members the grammar allows between real ones,
			// so they are skipped rather than misread as heritage.
			text := strings.TrimSpace(r.prog.Text(m))
			if text == "" || text == ";" {
				continue
			}
			// A static initialization block surfaces as an unnamed node whose text
			// starts with static and whose one child is the block; it is not heritage.
			// It is recorded as a static init step in its member position, alongside
			// any non-constant field initializers.
			if blk, ok := r.staticBlockBody(m); ok {
				if err := r.noteStaticInit(info, staticInitStep{block: blk}, taken); err != nil {
					return err
				}
				continue
			}
			if firstWord(text) != "extends" || info.base != nil {
				return &NotYetLowerable{Reason: "class heritage (" + firstWord(text) + ") is a later slice"}
			}
			base, err := r.baseClassOf(m)
			if err != nil {
				return err
			}
			info.base = base
			base.extended = true
		default:
			return &NotYetLowerable{Reason: "this class member kind is a later slice"}
		}
	}
	// Parameter properties are fields the constructor declares, and TypeScript
	// assigns them before the declared initializers run, so they sit first in
	// field order. The checker rejects a parameter property duplicating a
	// declared member, so the clash checks here are defense in depth.
	for _, f := range paramProps {
		if _, dup := info.fieldByName(f.prop); dup {
			return &NotYetLowerable{Reason: "a parameter property named like a field is a later slice"}
		}
		if _, c := info.methodByName(f.prop); c {
			return &NotYetLowerable{Reason: "a parameter property named like a method is a later slice"}
		}
	}
	info.fields = append(paramProps, info.fields...)
	// An abstract method is virtual by declaration: it claims its vtable slot
	// here, before any subclass registers, so the vtable exists even when no
	// override is in this module. Only the root may declare one; an abstract
	// method further down would need a slot the root's vtable does not have,
	// the same mid-chain gap an override of a mid-chain method hits.
	for _, m := range info.methods {
		if !m.abstract {
			continue
		}
		if info.base != nil {
			return &NotYetLowerable{Reason: "class " + name + " declares abstract ." + m.prop + " below the hierarchy root; a mid-chain virtual method is a later slice"}
		}
		if info.vprops == nil {
			info.vprops = map[string]bool{}
		}
		info.vprops[m.prop] = true
	}
	if err := r.checkAccessorClashes(info); err != nil {
		return err
	}
	if info.base != nil {
		if err := r.checkBaseClashes(info); err != nil {
			return err
		}
		if info.ctor == nil {
			// The synthesized derived constructor is the base's: same parameters,
			// passed straight through to super, exactly what JavaScript runs when a
			// derived class declares no constructor of its own.
			info.ctorParams = info.base.ctorParams
		} else if err := r.validateSuper(info); err != nil {
			return err
		}
	}

	info.recv = r.receiverName(info)
	r.classes[name] = info
	r.classOrder = append(r.classOrder, name)
	return nil
}

// checkAccessorClashes rejects a class whose accessors collide with the names
// this lowering mints. An accessor sharing a name with a field or a method
// would give one property two Go spellings, and a declared method named SetX
// would collide with the setter's emitted method.
func (r *Renderer) checkAccessorClashes(info *classInfo) error {
	for _, g := range info.getters {
		if _, c := info.fieldByName(g.prop); c {
			return &NotYetLowerable{Reason: "an accessor named like a field is a later slice"}
		}
		if _, c := info.methodByName(g.prop); c {
			return &NotYetLowerable{Reason: "an accessor named like a method is a later slice"}
		}
	}
	for _, s := range info.setters {
		if _, c := info.fieldByName(s.prop); c {
			return &NotYetLowerable{Reason: "an accessor named like a field is a later slice"}
		}
		if _, c := info.methodByName(s.prop); c {
			return &NotYetLowerable{Reason: "an accessor named like a method is a later slice"}
		}
		for _, m := range info.methods {
			if m.goName == s.goName {
				return &NotYetLowerable{Reason: "a method named " + m.prop + " collides with the ." + s.prop + " setter's Go name"}
			}
		}
	}
	return nil
}

// baseClassOf resolves an extends clause to the registered base class. The
// clause wraps one expression node whose child is the base; a plain identifier
// and a parenthesized one this slice resolves, and the other shapes each keep
// their own named reason. A generic base (extends Base<T>) carries
// type-argument children and needs monomorphization; extends null and a mixin
// call (extends f()) each name a construct this slice does not build. The
// identifier resolves the way a class name reference does, through its symbol to
// the exact registered declaration; the checker rejects a class used before its
// declaration, so a resolvable base is always already registered when the
// derived class reads it.
func (r *Renderer) baseClassOf(clause frontend.Node) (*classInfo, error) {
	kids := r.prog.Children(clause)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "an extends clause that is not a single base class is a later slice"}
	}
	ekids := r.prog.Children(kids[0])
	// A generic base carries the type arguments as extra children after the name,
	// so more than one child is the monomorphization case, the same reason the
	// generic method path names.
	if len(ekids) > 1 {
		return nil, &NotYetLowerable{Reason: "a generic base class needs monomorphization, a later slice"}
	}
	if len(ekids) != 1 {
		return nil, &NotYetLowerable{Reason: "a base class that is not a plain class name is a later slice"}
	}
	// A parenthesized base, extends (Base), is the name inside dressed in
	// syntactic parens; unwrap them and resolve the name the same way a bare one
	// resolves.
	base := ekids[0]
	for base.Kind() == frontend.NodeParenthesizedExpression {
		inner := r.prog.Children(base)
		if len(inner) != 1 {
			return nil, &NotYetLowerable{Reason: "a base class that is not a plain class name is a later slice"}
		}
		base = inner[0]
	}
	switch base.Kind() {
	case frontend.NodeIdentifier:
		info, ok := r.classNameRef(base)
		if !ok {
			return nil, &NotYetLowerable{Reason: "extending " + r.prog.Text(base) + ", which is not a class this slice lowers, is a later slice"}
		}
		return info, nil
	case frontend.NodeNullKeyword:
		return nil, &NotYetLowerable{Reason: "extends null is a later slice"}
	case frontend.NodeCallExpression:
		return nil, &NotYetLowerable{Reason: "a mixin base expression is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "a base class that is not a plain class name is a later slice"}
	}
}

// classMember is one emitted member for the clash walk: its kind, its source
// property, and the phrase an error names it by.
type classMember struct {
	kind string // "field", "method", or "accessor"
	prop string
	desc string
}

// classMembers indexes a class's own members by emitted Go name, the namespace
// a collision would corrupt.
func classMembers(c *classInfo) map[string]classMember {
	m := map[string]classMember{}
	for _, f := range c.fields {
		m[f.goName] = classMember{"field", f.prop, "field ." + f.prop}
	}
	for _, meth := range c.methods {
		m[meth.goName] = classMember{"method", meth.prop, "method ." + meth.prop}
	}
	for _, g := range c.getters {
		m[g.goName] = classMember{"accessor", g.prop, "accessor ." + g.prop}
	}
	for _, s := range c.setters {
		m[s.goName] = classMember{"accessor", s.prop, "accessor ." + s.prop}
	}
	return m
}

// checkBaseClashes walks a derived class's own members against the base chain.
// A method redeclaring a base method of the same name is an override and is
// recorded as a vtable slot below; every other collision hands back. Comparing
// the emitted Go names subsumes the property-name check (a property's Go name
// is its exported spelling) and also catches a cross-case collision like a
// field Total under a base method total, where one Go selector would mean two
// source properties; the same-prop condition on the method arm keeps such a
// cross-case pair out of the override path. The direct base's type name is
// reserved too, in both directions: it is the embedded field's name in the
// derived struct, and a member spelling it would shadow the embedded value Go
// promotion reads through.
func (r *Renderer) checkBaseClashes(info *classInfo) error {
	own := classMembers(info)
	if o, ok := own[info.base.goName]; ok {
		return &NotYetLowerable{Reason: "class " + info.name + "'s " + o.desc + " spells " + info.base.goName + ", the embedded base field's name"}
	}
	for b := info.base; b != nil; b = b.base {
		theirs := classMembers(b)
		for goName, o := range own {
			their, ok := theirs[goName]
			if !ok {
				continue
			}
			if o.kind == "method" && their.kind == "method" && o.prop == their.prop {
				continue // an override, recorded by recordOverrides below
			}
			return &NotYetLowerable{Reason: "class " + info.name + "'s " + o.desc + " collides with base class " + b.name + "'s " + their.desc + "; only a method overriding a same-named base method lowers"}
		}
		if their, ok := theirs[info.base.goName]; ok {
			return &NotYetLowerable{Reason: "base class " + b.name + "'s " + their.desc + " spells " + info.base.goName + ", the embedded base field's name in class " + info.name}
		}
	}
	return r.recordOverrides(info)
}

// recordOverrides registers each own method that redeclares a base-chain
// method as a virtual override: the hierarchy root gains a vtable slot for the
// method and this class is marked as filling it. The declaring class must be
// the root itself, because the vtable pointer lives on the root's struct and
// its slots take the root's type; a method introduced partway down the chain
// would need a second vtable on the mid class, machinery a later slice adds.
func (r *Renderer) recordOverrides(info *classInfo) error {
	root := info.base
	for root.base != nil {
		root = root.base
	}
	for _, m := range info.methods {
		var owner *classInfo
		for b := info.base; b != nil; b = b.base {
			if _, ok := b.methodByName(m.prop); ok {
				owner = b
			}
		}
		if owner == nil {
			continue
		}
		if owner != root {
			return &NotYetLowerable{Reason: "class " + info.name + " overrides ." + m.prop + ", which " + owner.name + " declares below the hierarchy root; a mid-chain virtual method is a later slice"}
		}
		if root.vprops == nil {
			root.vprops = map[string]bool{}
		}
		root.vprops[m.prop] = true
		if info.overrides == nil {
			info.overrides = map[string]bool{}
		}
		info.overrides[m.prop] = true
	}
	return nil
}

// validateSuper checks a derived constructor's super call: exactly one in the
// whole constructor (a conditional or repeated super has no single point to
// become the base assignment), and an argument count matching the base
// constructor. TypeScript permits this-free statements before super(), and the
// lowering runs them before the base assignment the way JavaScript does; a
// pre-super statement that reads this or super, or that declares a binding a
// later statement could read, stays a later slice. The argument nodes and the
// pre-super statements are kept for the constructor body.
func (r *Renderer) validateSuper(info *classInfo) error {
	if n := r.countSuperCalls(info.ctor); n != 1 {
		return &NotYetLowerable{Reason: "a derived constructor that calls super() " + itoa(n) + " times is a later slice"}
	}
	var block frontend.Node
	for _, k := range r.prog.Children(info.ctor) {
		if k.Kind() == frontend.NodeBlock {
			block = k
		}
	}
	stmts := r.prog.Children(block)
	superIdx := -1
	for i, s := range stmts {
		if _, ok := r.superCallOf(s); ok {
			superIdx = i
			break
		}
	}
	if superIdx < 0 {
		return &NotYetLowerable{Reason: "a derived constructor with no super() statement is a later slice"}
	}
	// A statement before super() runs while this is still in the temporal dead
	// zone, so one that reads this or super is the JavaScript error case and stays
	// declined; one that declares a binding a later statement could read is
	// declined too, because the pre-super statements lower in their own place
	// before the base assignment rather than through the body's var-hoist scope.
	for _, s := range stmts[:superIdx] {
		if subtreeHasKind(r.prog, s, frontend.NodeThisKeyword) {
			return &NotYetLowerable{Reason: "a statement that reads this before super() is a later slice"}
		}
		if subtreeHasKind(r.prog, s, frontend.NodeSuperKeyword) {
			return &NotYetLowerable{Reason: "a statement that reads super before super() is a later slice"}
		}
		if subtreeHasKind(r.prog, s, frontend.NodeVariableStatement) {
			return &NotYetLowerable{Reason: "a variable declaration before super() is a later slice"}
		}
	}
	args, _ := r.superCallOf(stmts[superIdx])
	if len(args) != len(info.base.ctorParams) {
		return &NotYetLowerable{Reason: "super() with an argument count that differs from the base constructor is a later slice"}
	}
	info.superArgs = args
	info.preSuper = stmts[:superIdx]
	return nil
}

// superCallOf recognizes a statement of the exact shape super(args) and
// returns the argument nodes.
func (r *Renderer) superCallOf(stmt frontend.Node) ([]frontend.Node, bool) {
	if stmt.Kind() != frontend.NodeExpressionStatement {
		return nil, false
	}
	kids := r.prog.Children(stmt)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeCallExpression {
		return nil, false
	}
	ckids := r.prog.Children(kids[0])
	if len(ckids) == 0 || ckids[0].Kind() != frontend.NodeSuperKeyword {
		return nil, false
	}
	return ckids[1:], true
}

// countSuperCalls counts the bare super(...) calls under n. A super.m() call
// is a call over a property access, not over the super keyword itself, so it
// does not count.
func (r *Renderer) countSuperCalls(n frontend.Node) int {
	count := 0
	if n.Kind() == frontend.NodeCallExpression {
		if kids := r.prog.Children(n); len(kids) > 0 && kids[0].Kind() == frontend.NodeSuperKeyword {
			count++
		}
	}
	for _, c := range r.prog.Children(n) {
		count += r.countSuperCalls(c)
	}
	return count
}

// memberIsStatic reports whether a class member carries the static modifier,
// which surfaces as its own unnamed child before the member's name.
func (r *Renderer) memberIsStatic(m frontend.Node) bool {
	kids := r.prog.Children(m)
	return len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown &&
		strings.TrimSpace(r.prog.Text(kids[0])) == "static"
}

// memberHasMod reports whether the member carries the named modifier, which
// the frontend surfaces as one unnamed node per keyword before the name.
func (r *Renderer) memberHasMod(m frontend.Node, mod string) bool {
	for _, k := range r.prog.Children(m) {
		if k.Kind() != frontend.NodeUnknown {
			return false
		}
		if strings.TrimSpace(r.prog.Text(k)) == mod {
			return true
		}
	}
	return false
}

// memberName reads the property a class member's name node spells, for the
// three static-name shapes this slice lowers: a plain identifier (m() {}), a
// string-literal name ("my method"() {}), and a computed name whose expression
// is a string literal (["m"]() {}). A string name resolves its escapes the same
// way a string-literal element-access key does (stringLiteralKey), so a
// declaration and a bracket read of the member agree on the mangled Go
// spelling. It returns false for a computed name that is not a constant string,
// a [Symbol.x] or a [expr], which stays a later slice, and for a name that
// decodes to a lone surrogate no Go identifier could carry. A ["constructor"]
// computed name is an ordinary property named "constructor", a prototype method
// distinct from the class constructor, so it resolves here to that string like
// any other constant name.
func (r *Renderer) memberName(nameNode frontend.Node) (string, bool) {
	switch nameNode.Kind() {
	case frontend.NodeIdentifier:
		return r.prog.Text(nameNode), true
	case frontend.NodeStringLiteral:
		return r.stringLiteralKey(nameNode)
	case frontend.NodeUnknown:
		// A computed name [expr] surfaces as an unnamed node wrapping the
		// expression; only a lone string-literal expression is a constant name.
		kids := r.prog.Children(nameNode)
		if len(kids) == 1 && kids[0].Kind() == frontend.NodeStringLiteral {
			return r.stringLiteralKey(kids[0])
		}
	}
	return "", false
}

// isPrivateName reports whether a class member's name node is a JavaScript
// private name (#x). The parser surfaces a private name as a childless unnamed
// token whose text leads with #, distinct from the NodeIdentifier a public
// member carries and from the expression-bearing unnamed node a computed name
// wraps, so those two shapes are told apart by the child count and the leading
// rune.
func (r *Renderer) isPrivateName(n frontend.Node) bool {
	return n.Kind() == frontend.NodeUnknown &&
		len(r.prog.Children(n)) == 0 &&
		strings.HasPrefix(strings.TrimSpace(r.prog.Text(n)), "#")
}

// memberNameReason is the handback a reader returns when memberName declines the
// name node: a computed name that is not a constant string names itself, so the
// leftover reason after this slice is honest about what remains; any other
// shape (a numeric literal name, an unreadable node) keeps the family's
// "without a plain identifier name" phrasing, with kind naming the member.
func (r *Renderer) memberNameReason(nameNode frontend.Node, kind string) error {
	if r.isPrivateName(nameNode) {
		// A private name that reaches a reader is one this slice does not lower (a
		// private static field, a private accessor); it names itself so the leftover
		// reason is honest about what remains rather than the computed-name phrasing
		// a #x would otherwise fall into.
		return &NotYetLowerable{Reason: "a private " + kind + " is a later slice"}
	}
	if nameNode.Kind() == frontend.NodeUnknown {
		return &NotYetLowerable{Reason: "a computed member name that is not a constant string is a later slice"}
	}
	return &NotYetLowerable{Reason: "a " + kind + " without a plain identifier name is a later slice"}
}

// classFieldOf reads one property declaration into a classField. The member's
// children are the name, an optional type annotation (an unnamed node), and an
// optional initializer expression. An initializer bento's node vocabulary does
// not name would be indistinguishable from an annotation, so a member whose
// source carries an = that did not surface as an initializer hands back rather
// than silently dropping the value; a definite-assignment assertion (x!: T)
// waives the checker's initialization proof this lowering relies on, so it
// hands back too.
func (r *Renderer) classFieldOf(m frontend.Node) (classField, error) {
	text := strings.TrimSpace(r.prog.Text(m))
	if w := firstWord(text); w == "static" || w == "declare" || w == "accessor" {
		return classField{}, &NotYetLowerable{Reason: "a " + w + " class field is a later slice"}
	}
	if r.memberHasMod(m, "abstract") {
		// An abstract field has no runtime presence on the base; the concrete
		// declaration lives on the subclass, a layout this slice does not build.
		return classField{}, &NotYetLowerable{Reason: "an abstract field is a later slice"}
	}
	kids := r.prog.Children(m)
	if len(kids) == 0 {
		return classField{}, &NotYetLowerable{Reason: "a class field without a plain identifier name is a later slice"}
	}
	var prop, goName string
	var ok bool
	if r.isPrivateName(kids[0]) {
		// A private field lowers to an unexported p_-prefixed struct field; the JS
		// name (#x) stays the property so a this.#x read or write resolves to it.
		prop = strings.TrimSpace(r.prog.Text(kids[0]))
		goName, ok = privateGoName(prop)
		if !ok {
			return classField{}, &NotYetLowerable{Reason: "private field name is not a Go identifier"}
		}
	} else {
		prop, ok = r.memberName(kids[0])
		if !ok {
			return classField{}, r.memberNameReason(kids[0], "class field")
		}
		goName, ok = exportedField(prop)
		if !ok {
			return classField{}, &NotYetLowerable{Reason: "class field name is not a Go identifier"}
		}
	}
	var init frontend.Node
	if last := kids[len(kids)-1]; len(kids) > 1 && last.Kind() != frontend.NodeUnknown {
		init = last
	}
	if init == nil && strings.Contains(text, "=") {
		return classField{}, &NotYetLowerable{Reason: "class field initializer bento could not read is a later slice"}
	}
	if strings.Contains(strings.SplitN(text, "=", 2)[0], "!") {
		return classField{}, &NotYetLowerable{Reason: "a definite-assignment field (x!: T) is a later slice"}
	}
	return classField{prop: prop, goName: goName, ident: kids[0], init: init}, nil
}

// ctorParamsOf validates the constructor's shape and returns its parameter
// nodes plus the fields its parameter properties declare. A default value
// needs call-site defaulting, a later slice.
func (r *Renderer) ctorParamsOf(ctor frontend.Node) ([]frontend.Node, []classField, error) {
	var params []frontend.Node
	var props []classField
	hasBody := false
	for _, k := range r.prog.Children(ctor) {
		switch k.Kind() {
		case frontend.NodeParameter:
			prop, err := r.ctorParam(k)
			if err != nil {
				return nil, nil, err
			}
			params = append(params, k)
			if prop != nil {
				props = append(props, *prop)
			}
		case frontend.NodeBlock:
			hasBody = true
		}
	}
	if !hasBody {
		return nil, nil, &NotYetLowerable{Reason: "a constructor overload signature is a later slice"}
	}
	return params, props, nil
}

// ctorParam validates one constructor parameter and, when it is a parameter
// property (constructor(public x: number)), returns the field it declares.
// The field's init is the parameter's own name node: the property's whole
// effect is this.x = x, so it lowers like a declared field initialized to the
// parameter, and being a bare name it always folds pure.
func (r *Renderer) ctorParam(p frontend.Node) (*classField, error) {
	kids := r.prog.Children(p)
	mods := 0
	for _, k := range kids {
		if k.Kind() != frontend.NodeUnknown {
			break
		}
		switch text := strings.TrimSpace(r.prog.Text(k)); text {
		case "public", "private", "protected", "readonly":
			mods++
		default:
			return nil, &NotYetLowerable{Reason: "the parameter modifier " + text + " is a later slice"}
		}
	}
	if mods >= len(kids) || kids[mods].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "a parameter that is not a plain identifier is a later slice"}
	}
	nameNode := kids[mods]
	for _, extra := range kids[mods+1:] {
		if extra.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: "a parameter with a default value is a later slice"}
		}
	}
	if mods == 0 {
		return nil, nil
	}
	prop := r.prog.Text(nameNode)
	goName, ok := exportedField(prop)
	if !ok {
		return nil, &NotYetLowerable{Reason: "parameter property name is not a Go identifier"}
	}
	return &classField{prop: prop, goName: goName, ident: nameNode, init: nameNode}, nil
}

// paramNameNode returns a parameter's name node, past the leading modifier
// children a parameter property carries.
func (r *Renderer) paramNameNode(p frontend.Node) frontend.Node {
	for _, k := range r.prog.Children(p) {
		if k.Kind() == frontend.NodeIdentifier {
			return k
		}
	}
	return r.prog.Children(p)[0]
}

// plainParam checks one parameter node is a bare typed identifier: the name,
// then at most an unnamed type annotation. A modifier (a parameter property), a
// default value, a rest element, or a binding pattern all hand back.
func (r *Renderer) plainParam(p frontend.Node) error {
	if w := firstWord(strings.TrimSpace(r.prog.Text(p))); w == "public" || w == "private" || w == "protected" || w == "readonly" {
		return &NotYetLowerable{Reason: "a parameter property (constructor(" + w + " x)) is a later slice"}
	}
	kids := r.prog.Children(p)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "a parameter that is not a plain identifier is a later slice"}
	}
	for _, extra := range kids[1:] {
		if extra.Kind() != frontend.NodeUnknown {
			return &NotYetLowerable{Reason: "a parameter with a default value is a later slice"}
		}
	}
	return nil
}

// classMethodOf reads one method declaration into a classMethod.
func (r *Renderer) classMethodOf(m frontend.Node) (classMethod, error) {
	// Modifiers surface as unnamed nodes before the name. Only abstract is
	// modeled here; any other modifier changes semantics this slice does not
	// build (async a coroutine, an access keyword a lowercase spelling) and
	// hands back by its keyword.
	kids := r.prog.Children(m)
	abstract := false
	generator := false
	async := false
	private := ""
	for len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown {
		// A computed name [expr] surfaces as an unnamed node that wraps the
		// expression, so it carries a child; a modifier (abstract, async, the
		// generator star) is a childless keyword token. The name is not a
		// modifier, so stop stripping once the unnamed node holds an expression
		// and let the memberName read below claim or narrow it.
		if len(r.prog.Children(kids[0])) > 0 {
			break
		}
		w := strings.TrimSpace(r.prog.Text(kids[0]))
		// A private name (#m) surfaces as a childless token like a modifier but is
		// the member's name, not a keyword: record it and stop stripping, leaving
		// the token in place so the body scan below still sees the whole member.
		if strings.HasPrefix(w, "#") {
			private = w
			break
		}
		switch w {
		case "abstract":
			abstract = true
		case "*":
			// The generator star is a recognized marker, not a decline: it flags
			// the method to lower as a state-machine closure (generatorMethodDecl)
			// the way abstract flags the body-less form.
			generator = true
		case "async":
			// An async modifier is a recognized marker: an await-free async method
			// lowers to a synchronous method returning a settled promise
			// (asyncMethodDecl), so async flags that path the way the generator star
			// flags its own.
			async = true
		default:
			return classMethod{}, &NotYetLowerable{Reason: "a " + w + " method is a later slice"}
		}
		kids = kids[1:]
	}
	if async && generator {
		// An async generator (async *) yields promises through an async iterator, a
		// shape neither the generator nor the async lowering models; it keeps its own
		// reason rather than lowering as one or the other.
		return classMethod{}, &NotYetLowerable{Reason: "an async generator method is a later slice"}
	}
	if len(kids) == 0 {
		return classMethod{}, &NotYetLowerable{Reason: "a method without a plain identifier name is a later slice"}
	}
	var prop, goName string
	var ok bool
	if private != "" {
		// A private method lowers to an unexported p_-prefixed Go method; the JS
		// name (#m) stays the property so a this.#m() call resolves to it and #m and
		// m coexist as p_m and M.
		prop = private
		goName, ok = privateGoName(private)
		if !ok {
			return classMethod{}, &NotYetLowerable{Reason: "private method name is not a Go identifier"}
		}
	} else if r.isSymbolIteratorName(kids[0]) {
		// A [Symbol.iterator] member is the iterable protocol's entry point: it
		// lowers to a Go method under the fixed name the for...of lowering calls to
		// obtain the iterator, so its well-known computed name resolves where an
		// ordinary [expr] name would hand back.
		prop = symbolIteratorProp
		goName = symbolIteratorGoName
	} else if r.isSymbolAsyncIteratorName(kids[0]) {
		// A [Symbol.asyncIterator] member is the async iterable protocol's entry
		// point: it lowers to a Go method under the fixed name the for await...of
		// lowering calls to obtain the async iterator, the async mirror of the
		// [Symbol.iterator] case.
		prop = symbolAsyncIteratorProp
		goName = symbolAsyncIteratorGoName
	} else {
		prop, ok = r.memberName(kids[0])
		if !ok {
			return classMethod{}, r.memberNameReason(kids[0], "method")
		}
		goName, ok = exportedField(prop)
		if !ok {
			return classMethod{}, &NotYetLowerable{Reason: "method name is not a Go identifier"}
		}
	}
	// A body-less non-abstract declaration is an overload signature, whose
	// call-site selection is a later slice; an abstract method is exactly the
	// body-less form, its body being the slot a subclass fills.
	hasBody := false
	for _, k := range kids {
		if k.Kind() == frontend.NodeBlock {
			hasBody = true
		}
	}
	if !abstract && !hasBody {
		return classMethod{}, &NotYetLowerable{Reason: "a method overload signature is a later slice"}
	}
	return classMethod{prop: prop, goName: goName, node: m, abstract: abstract, generator: generator, async: async}, nil
}

// staticFieldOf reads one static field into a classField whose goName is the
// package var it becomes. The initializer is required: a package var runs its
// initializer before main, so an uninitialized static's zero value matches
// JavaScript's undefined for no type, and the initializer must be a this-free,
// name-free constant expression, because main's locals are not in scope at
// package level and no evaluation order exists between the vars.
func (r *Renderer) staticFieldOf(info *classInfo, m frontend.Node, taken map[string]bool) (classField, error) {
	kids := r.prog.Children(m)
	if len(kids) < 2 {
		return classField{}, &NotYetLowerable{Reason: "a static field without a plain identifier name is a later slice"}
	}
	prop, ok := r.memberName(kids[1])
	if !ok {
		return classField{}, r.memberNameReason(kids[1], "static field")
	}
	propGo, ok := exportedField(prop)
	if !ok {
		return classField{}, &NotYetLowerable{Reason: "static field name is not a Go identifier"}
	}
	var init frontend.Node
	if last := kids[len(kids)-1]; len(kids) > 2 && last.Kind() != frontend.NodeUnknown {
		init = last
	}
	// A constant initializer stays the package var's own initializer, the readable
	// common case. A non-constant one runs in the class's static init function in
	// member order, so it may read an earlier static or call a module function; an
	// initializer that reads this reaches the class constructor object, a
	// dynamic-world value this slice does not model, so it stays declined.
	runtimeInit := false
	switch {
	case init == nil:
		// A static field with no initializer defaults to undefined. An untyped
		// (implicit-any) static lowers to a boxed value.Value whose zero value is
		// undefined, so the package var declares zero-valued with no static-init
		// step and reads back undefined. A field with a concrete type annotation
		// would zero to that type's Go zero (0, ""), not undefined, so it stays a
		// later slice rather than miscompile the default.
		if r.prog.TypeAt(kids[1]).Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return classField{}, &NotYetLowerable{Reason: "a typed static field without an initializer is a later slice"}
		}
	case !r.pureStaticInit(init):
		if subtreeHasKind(r.prog, init, frontend.NodeThisKeyword) {
			return classField{}, &NotYetLowerable{Reason: "a static field initializer that reads this is a later slice"}
		}
		if subtreeHasKind(r.prog, init, frontend.NodeSuperKeyword) {
			return classField{}, &NotYetLowerable{Reason: "a static field initializer that reads super is a later slice"}
		}
		runtimeInit = true
	}
	name := ""
	for _, cand := range []string{lowerFirst(info.goName) + propGo, info.goName + propGo} {
		if !taken[cand] && !goKeywords[cand] {
			name = cand
			break
		}
	}
	if name == "" {
		return classField{}, &NotYetLowerable{Reason: "no free package name for static ." + prop + " of class " + info.name}
	}
	taken[name] = true
	return classField{prop: prop, goName: name, ident: kids[1], init: init, runtimeInit: runtimeInit}, nil
}

// pureStaticInit reports whether a static initializer may run as a package var
// initializer: pureCtorValue minus names, since an identifier at package level
// would resolve against nothing (main's locals) or against another var with no
// defined order between them.
func (r *Renderer) pureStaticInit(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeNumericLiteral, frontend.NodeStringLiteral,
		frontend.NodeBigIntLiteral, frontend.NodeNoSubstitutionTemplateLiteral,
		frontend.NodeTrueKeyword, frontend.NodeFalseKeyword:
		return true
	case frontend.NodeParenthesizedExpression, frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.pureStaticInit(kids[0])
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return false
		}
		if op := r.prog.Text(parts[1]); op == "=" {
			return false
		} else if _, compound := compoundBaseOp(op); compound {
			return false
		}
		return r.pureStaticInit(parts[0]) && r.pureStaticInit(parts[2])
	default:
		return false
	}
}

// staticMethodOf reads one static method into a classMethod whose goName is
// the package function it becomes, the class name prefixed the way a Go
// package spells a type's related functions (ABump).
func (r *Renderer) staticMethodOf(info *classInfo, m frontend.Node, taken map[string]bool) (classMethod, error) {
	// Modifiers surface as unnamed nodes before the name, the same shape
	// classMethodOf strips. A static method always leads with the static keyword;
	// async marks the promise-returning path asyncStaticFuncDecl takes, and any other
	// modifier keeps its own reason. The name follows once an unnamed node either
	// runs out or holds a computed expression.
	kids := r.prog.Children(m)
	async := false
	generator := false
	for len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown {
		if len(r.prog.Children(kids[0])) > 0 {
			break // a computed name [expr] wrapper, not a modifier
		}
		w := strings.TrimSpace(r.prog.Text(kids[0]))
		switch w {
		case "static":
			// The static keyword is implied by this being a static member; skip it.
		case "async":
			async = true
		case "*":
			generator = true
		default:
			return classMethod{}, &NotYetLowerable{Reason: "a static " + w + " method is a later slice"}
		}
		kids = kids[1:]
	}
	if generator {
		// A static generator (async or plain) is neither the state-machine closure
		// generatorMethodDecl builds for an instance nor the settled promise the async
		// path returns; it keeps its own reason.
		if async {
			return classMethod{}, &NotYetLowerable{Reason: "a static async generator method is a later slice"}
		}
		return classMethod{}, &NotYetLowerable{Reason: "a static generator method is a later slice"}
	}
	if len(kids) == 0 {
		return classMethod{}, &NotYetLowerable{Reason: "a static method without a plain identifier name is a later slice"}
	}
	prop, ok := r.memberName(kids[0])
	if !ok {
		return classMethod{}, r.memberNameReason(kids[0], "static method")
	}
	propGo, ok := exportedField(prop)
	if !ok {
		return classMethod{}, &NotYetLowerable{Reason: "static method name is not a Go identifier"}
	}
	name := info.goName + propGo
	if taken[name] || goKeywords[name] {
		return classMethod{}, &NotYetLowerable{Reason: "the module already speaks " + name + ", the name static ." + prop + " needs"}
	}
	taken[name] = true
	return classMethod{prop: prop, goName: name, node: m, async: async}, nil
}

// getterOf reads one get accessor into a classMethod; it emits through the
// same method path an ordinary method takes, since a getter is a method whose
// call the source spells as a read.
func (r *Renderer) getterOf(m frontend.Node) (classMethod, error) {
	if r.memberHasMod(m, "abstract") {
		return classMethod{}, &NotYetLowerable{Reason: "an abstract accessor is a later slice"}
	}
	kids := r.prog.Children(m)
	if len(kids) == 0 {
		return classMethod{}, &NotYetLowerable{Reason: "a get accessor without a plain identifier name is a later slice"}
	}
	prop, ok := r.memberName(kids[0])
	if !ok {
		return classMethod{}, r.memberNameReason(kids[0], "get accessor")
	}
	goName, ok := exportedField(prop)
	if !ok {
		return classMethod{}, &NotYetLowerable{Reason: "accessor name is not a Go identifier"}
	}
	return classMethod{prop: prop, goName: goName, node: m}, nil
}

// setterOf reads one set accessor into a classSetter. The single parameter
// must be a plain typed identifier, the same rule a constructor parameter
// keeps, and its name node is retained so a store coerces the value against
// the declared parameter type.
func (r *Renderer) setterOf(m frontend.Node) (classSetter, error) {
	if r.memberHasMod(m, "abstract") {
		return classSetter{}, &NotYetLowerable{Reason: "an abstract accessor is a later slice"}
	}
	kids := r.prog.Children(m)
	if len(kids) == 0 {
		return classSetter{}, &NotYetLowerable{Reason: "a set accessor without a plain identifier name is a later slice"}
	}
	prop, ok := r.memberName(kids[0])
	if !ok {
		return classSetter{}, r.memberNameReason(kids[0], "set accessor")
	}
	propGo, ok := exportedField(prop)
	if !ok {
		return classSetter{}, &NotYetLowerable{Reason: "accessor name is not a Go identifier"}
	}
	var param frontend.Node
	for _, k := range kids[1:] {
		if k.Kind() != frontend.NodeParameter {
			continue
		}
		if param != nil {
			return classSetter{}, &NotYetLowerable{Reason: "a set accessor with more than one parameter is a later slice"}
		}
		if err := r.plainParam(k); err != nil {
			return classSetter{}, err
		}
		param = r.prog.Children(k)[0]
	}
	if param == nil {
		return classSetter{}, &NotYetLowerable{Reason: "a set accessor without a parameter is a later slice"}
	}
	return classSetter{prop: prop, goName: "Set" + propGo, node: m, param: param}, nil
}

// staticAccessorName strips a static accessor's leading modifiers and reads its
// property name, the shared front of staticGetterOf and staticSetterOf. The
// static keyword is expected and dropped; any other modifier (an access
// modifier the runtime does not model) keeps its own reason, and a private name
// (static get #x) is left for memberName to decline with the private phrasing
// rather than being mistaken for a modifier. It returns the source property, the
// remaining children from the name onward, and the exported spelling the package
// function name is built from.
func (r *Renderer) staticAccessorName(m frontend.Node, kind string) (prop, propGo string, rest []frontend.Node, err error) {
	if r.memberHasMod(m, "abstract") {
		return "", "", nil, &NotYetLowerable{Reason: "an abstract accessor is a later slice"}
	}
	kids := r.prog.Children(m)
	for len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown && len(r.prog.Children(kids[0])) == 0 {
		w := strings.TrimSpace(r.prog.Text(kids[0]))
		if strings.HasPrefix(w, "#") {
			break // a private name, left for memberName to decline honestly
		}
		if w != "static" {
			return "", "", nil, &NotYetLowerable{Reason: "a static " + w + " accessor is a later slice"}
		}
		kids = kids[1:]
	}
	if len(kids) == 0 {
		return "", "", nil, &NotYetLowerable{Reason: "a " + kind + " without a plain identifier name is a later slice"}
	}
	prop, ok := r.memberName(kids[0])
	if !ok {
		return "", "", nil, r.memberNameReason(kids[0], kind)
	}
	propGo, ok = exportedField(prop)
	if !ok {
		return "", "", nil, &NotYetLowerable{Reason: "accessor name is not a Go identifier"}
	}
	return prop, propGo, kids, nil
}

// staticGetterOf reads one static get accessor into a classMethod whose goName
// is the package function it becomes, the class name prefixed the way
// staticMethodOf mints a static method's name (CX for static get x of class C).
// The name threads the same taken map, so a static getter colliding with a
// static field or method the module already speaks hands back with the
// established phrasing.
func (r *Renderer) staticGetterOf(info *classInfo, m frontend.Node, taken map[string]bool) (classMethod, error) {
	prop, propGo, _, err := r.staticAccessorName(m, "get accessor")
	if err != nil {
		return classMethod{}, err
	}
	name := info.goName + propGo
	if taken[name] || goKeywords[name] {
		return classMethod{}, &NotYetLowerable{Reason: "the module already speaks " + name + ", the name static get ." + prop + " needs"}
	}
	taken[name] = true
	return classMethod{prop: prop, goName: name, node: m}, nil
}

// staticSetterOf reads one static set accessor into a classSetter whose goName
// is the Set-prefixed package function it becomes (CSetX for static set x of
// class C), the static twin of setterOf's SetX method. The single parameter
// must be a plain typed identifier, and its name node is kept so a store coerces
// the value against the declared type. The name threads the taken map the same
// way staticGetterOf does.
func (r *Renderer) staticSetterOf(info *classInfo, m frontend.Node, taken map[string]bool) (classSetter, error) {
	prop, propGo, rest, err := r.staticAccessorName(m, "set accessor")
	if err != nil {
		return classSetter{}, err
	}
	name := info.goName + "Set" + propGo
	if taken[name] || goKeywords[name] {
		return classSetter{}, &NotYetLowerable{Reason: "the module already speaks " + name + ", the name static set ." + prop + " needs"}
	}
	var param frontend.Node
	for _, k := range rest[1:] {
		if k.Kind() != frontend.NodeParameter {
			continue
		}
		if param != nil {
			return classSetter{}, &NotYetLowerable{Reason: "a set accessor with more than one parameter is a later slice"}
		}
		if err := r.plainParam(k); err != nil {
			return classSetter{}, err
		}
		param = r.prog.Children(k)[0]
	}
	if param == nil {
		return classSetter{}, &NotYetLowerable{Reason: "a set accessor without a parameter is a later slice"}
	}
	taken[name] = true
	return classSetter{prop: prop, goName: name, node: m, param: param}, nil
}

// lowerFirst lowercases the first letter of a Go name, the package-var
// spelling of a class-owned name (aTotal for A.total).
func lowerFirst(s string) string {
	return strings.ToLower(s[:1]) + s[1:]
}

// receiverName picks the receiver identifier the constructor and every method
// share, the way a Go programmer names one: the first letter of the type,
// falling back to the lowercased type name and then to self when the short name
// is already spoken for by any identifier inside the class body (a parameter, a
// local, a global the body references), so the receiver never shadows a name a
// body reads.
func (r *Renderer) receiverName(info *classInfo) string {
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
	walk(info.decl)
	for _, cand := range []string{strings.ToLower(info.goName[:1]), strings.ToLower(info.goName), "self"} {
		if !taken[cand] && !goKeywords[cand] {
			return cand
		}
	}
	base := strings.ToLower(info.goName[:1])
	for n := 2; ; n++ {
		cand := base + itoa(n)
		if !taken[cand] {
			return cand
		}
	}
}

// classOfType resolves a type to the registered class it instantiates, by
// walking from the type to its declaring symbol and from the symbol to the
// declaration the registry recorded. The declaration must match, not just the
// name, so a class declared in a nested scope that shadows a top-level name is
// never confused with the registered one.
func (r *Renderer) classOfType(t frontend.Type) (*classInfo, bool) {
	if len(r.classes) == 0 || t.Flags&frontend.TypeObject == 0 {
		return nil, false
	}
	sym, ok := r.prog.TypeSymbol(t)
	if !ok || sym.Flags&frontend.SymbolClass == 0 {
		return nil, false
	}
	info, ok := r.classes[sym.Name]
	if !ok {
		return nil, false
	}
	for _, d := range r.prog.Declarations(sym) {
		if d == info.decl {
			return info, true
		}
	}
	return nil, false
}

// classOfNode resolves an expression node to the registered class its type
// instantiates.
func (r *Renderer) classOfNode(n frontend.Node) (*classInfo, bool) {
	return r.classOfType(r.prog.TypeAt(n))
}

// classReceiver resolves the receiver of a member access to a registered
// class: this inside a lowered class body resolves to that class, and any other
// expression resolves through its checker type.
func (r *Renderer) classReceiver(obj frontend.Node) (*classInfo, bool) {
	if obj.Kind() == frontend.NodeThisKeyword {
		return r.curClass, r.curClass != nil
	}
	return r.classOfNode(obj)
}

// classNameRef resolves an identifier that names a class itself, the A of new
// A() or of a static access A.total, to the registered class. The identifier's
// symbol must declare the exact registered declaration, so a nested class that
// shadows a top-level name, or a class spelled like a built-in, still resolves
// to what the name means at the use site. This is distinct from classOfNode:
// the checker types the identifier A as the class's own type, whose symbol is
// the same class symbol an instance type walks to, so an instance lookup on A
// would wrongly succeed; static routing therefore asks this question first.
func (r *Renderer) classNameRef(nameNode frontend.Node) (*classInfo, bool) {
	info, ok := r.classes[r.prog.Text(nameNode)]
	if !ok {
		return nil, false
	}
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok || sym.Flags&frontend.SymbolClass == 0 {
		return nil, false
	}
	for _, d := range r.prog.Declarations(sym) {
		if d == info.decl {
			return info, true
		}
	}
	return nil, false
}

// renderClasses emits the declarations of every registered class in source
// order: the struct, the static vars, the constructor, the methods, the
// accessors, then the static functions, the order a hand-written Go file keeps
// a type and its related declarations in.
func (r *Renderer) renderClasses() ([]ast.Decl, error) {
	var out []ast.Decl
	for _, name := range r.classOrder {
		info := r.classes[name]
		decls, err := r.renderClass(info)
		if err != nil {
			return nil, err
		}
		out = append(out, decls...)
	}
	return out, nil
}

func (r *Renderer) renderClass(info *classInfo) ([]ast.Decl, error) {
	var out []ast.Decl
	if len(info.vprops) > 0 {
		vt, err := r.vtableTypeDecl(info)
		if err != nil {
			return nil, err
		}
		out = append(out, vt)
	}
	structDecl, err := r.classStruct(info)
	if err != nil {
		return nil, err
	}
	out = append(out, structDecl)
	for _, f := range info.statics {
		vd, err := r.staticVarDecl(f)
		if err != nil {
			return nil, err
		}
		out = append(out, vd)
	}
	// The hierarchy root and every class with its own overrides carry a vtable
	// var holding the slot functions instances of that exact class dispatch
	// through; a class with no overrides of its own shares its nearest
	// ancestor's var.
	if len(info.vprops) > 0 || len(info.overrides) > 0 {
		vd, err := r.vtableVarDecl(info)
		if err != nil {
			return nil, err
		}
		out = append(out, vd)
	}
	ctorDecls, err := r.classCtor(info)
	if err != nil {
		return nil, err
	}
	out = append(out, ctorDecls...)
	for _, m := range info.methods {
		// A virtual method's body emits under its Impl name; the root also
		// gains the entry method under the original name, so every call site
		// keeps its spelling and dispatch runs through the vtable. An abstract
		// method is the entry alone: it has no body, its slot panics until a
		// concrete subclass's vtable fills it.
		name := m.goName
		if sig, ok := r.prog.SignatureAt(m.node); ok && len(sig.TypeParams) != 0 {
			// A generic method has no single Go form, since a Go method carries no type
			// parameter. It emits one mangled method per instantiation a call site fixed,
			// each body lowered with that instantiation's substitution active, and its
			// call sites already resolve to the matching mangled name.
			mds, err := r.genericMethodDecls(info, m)
			if err != nil {
				return nil, err
			}
			out = append(out, mds...)
			continue
		}
		if m.generator {
			// A generator lowers to a state-machine closure under its own name.
			// Its result type is the closure, not a plain value, so the vtable's
			// forwarding entry and Impl split does not model it yet; a generator
			// caught in a class hierarchy hands back rather than emit a mismatch.
			if info.vprops[m.prop] || info.overrides[m.prop] {
				return nil, &NotYetLowerable{Reason: "a generator method in a class hierarchy is a later slice"}
			}
			md, err := r.generatorMethodDecl(info, m)
			if err != nil {
				return nil, err
			}
			out = append(out, md)
			continue
		}
		if m.async {
			// An await-free async method lowers to a synchronous method returning a
			// settled promise. Its result type is a *value.Promise, not a plain value,
			// so like a generator it does not model the vtable's entry and Impl split
			// yet; an async method caught in a class hierarchy hands back rather than
			// emit a dispatch mismatch.
			if info.vprops[m.prop] || info.overrides[m.prop] {
				return nil, &NotYetLowerable{Reason: "an async method in a class hierarchy is a later slice"}
			}
			md, err := r.asyncMethodDecl(info, m, m.goName)
			if err != nil {
				return nil, err
			}
			out = append(out, md)
			continue
		}
		if info.vprops[m.prop] {
			entry, err := r.virtualEntryDecl(info, m)
			if err != nil {
				return nil, err
			}
			out = append(out, entry)
			if m.abstract {
				continue
			}
			name = implName(m)
		} else if info.overrides[m.prop] {
			name = implName(m)
		}
		md, err := r.classMethodDecl(info, m, name)
		if err != nil {
			return nil, err
		}
		out = append(out, md)
	}
	for _, g := range info.getters {
		md, err := r.classMethodDecl(info, g, g.goName)
		if err != nil {
			return nil, err
		}
		out = append(out, md)
	}
	// A setter emits through the same method path: its checker signature is the
	// one parameter and a void return, exactly the SetX method a Go author writes.
	for _, s := range info.setters {
		md, err := r.classMethodDecl(info, classMethod{prop: s.prop, goName: s.goName, node: s.node}, s.goName)
		if err != nil {
			return nil, err
		}
		out = append(out, md)
	}
	for _, m := range info.staticMethods {
		var fd ast.Decl
		var err error
		if m.async {
			fd, err = r.asyncStaticFuncDecl(m)
		} else {
			fd, err = r.staticFuncDecl(m)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, fd)
	}
	// A static accessor emits through the same package-function path a static
	// method takes: a getter's signature is no parameters and a T return, a
	// setter's is the one parameter and a void return, exactly the CX and CSetX
	// functions a read and a write route to.
	for _, g := range info.staticGetters {
		fd, err := r.staticFuncDecl(g)
		if err != nil {
			return nil, err
		}
		out = append(out, fd)
	}
	// Static initialization steps, blocks and non-constant field initializers in
	// member order, emit their lowered statements into a package function the main
	// body calls at the class declaration's position, the one place package-level
	// Go can run ordered work; the class declares no such function when every
	// static is a plain constant.
	if len(info.staticInit) > 0 {
		fd, err := r.staticInitFuncDecl(info)
		if err != nil {
			return nil, err
		}
		out = append(out, fd)
	}
	for _, s := range info.staticSetters {
		fd, err := r.staticFuncDecl(classMethod{prop: s.prop, goName: s.goName, node: s.node})
		if err != nil {
			return nil, err
		}
		out = append(out, fd)
	}
	if info.thrownAsError {
		out = append(out, r.thrownMethodDecls(info)...)
	}
	return out, nil
}

// staticInitName is the name of the package function a class's static
// initialization steps lower into, the function the main body calls at the
// class declaration's position.
func staticInitName(c *classInfo) string {
	return "staticInit" + c.goName
}

// staticBlockBody reports whether a class member node is a static
// initialization block and returns its statement block. A static block surfaces
// as an unnamed node whose text starts with static and whose one child is the
// block, which is how it tells apart from a static modifier token (childless)
// and from the extends and implements heritage clauses.
func (r *Renderer) staticBlockBody(m frontend.Node) (frontend.Node, bool) {
	if firstWord(strings.TrimSpace(r.prog.Text(m))) != "static" {
		return nil, false
	}
	kids := r.prog.Children(m)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeBlock {
		return nil, false
	}
	return kids[0], true
}

// staticInitFuncDecl lowers a class's static initialization blocks into one
// package function, the blocks concatenated in member order the way JavaScript
// runs them at class-definition time. A block that reads this or super touches
// the class constructor object, a dynamic-world value this slice does not model,
// so it hands back rather than drop the reference.
func (r *Renderer) staticInitFuncDecl(info *classInfo) (ast.Decl, error) {
	var stmts []ast.Stmt
	for _, step := range info.staticInit {
		if step.field != nil {
			asn, err := r.staticInitAssign(*step.field)
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, asn)
			continue
		}
		blk := step.block
		if subtreeHasKind(r.prog, blk, frontend.NodeThisKeyword) {
			return nil, &NotYetLowerable{Reason: "a static block that reads this is a later slice"}
		}
		if subtreeHasKind(r.prog, blk, frontend.NodeSuperKeyword) {
			return nil, &NotYetLowerable{Reason: "a static block that reads super is a later slice"}
		}
		body, err := r.scopedBlock(blk, 0)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, body.List...)
	}
	return &ast.FuncDecl{
		Name: ident(staticInitName(info)),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: stmts},
	}, nil
}

// staticInitAssign lowers a non-constant static field initializer into the
// assignment that runs in the class's static init function: the package var on
// the left, the initializer coerced to the field's declared type on the right,
// so a static reading an earlier static or calling a module function evaluates
// where JavaScript evaluates it. An initializer that reaches a not-yet-lowerable
// construct hands back through the ordinary expression path, no special casing.
func (r *Renderer) staticInitAssign(f classField) (ast.Stmt, error) {
	rhs, err := r.lowerExpr(f.init)
	if err != nil {
		return nil, err
	}
	rhs, err = r.coerceToTarget(rhs, f.init, f.ident)
	if err != nil {
		return nil, err
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ident(f.goName)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{rhs},
	}, nil
}

// noteStaticInit appends a static initialization step, minting the class's init
// function name on the first step so a clash with an existing package-level name
// hands back rather than redeclare it. The first block or non-constant field
// initializer a class has is what mints the name; a class of only constant
// statics has no step and no init function.
func (r *Renderer) noteStaticInit(info *classInfo, step staticInitStep, taken map[string]bool) error {
	if len(info.staticInit) == 0 {
		if nm := staticInitName(info); taken[nm] {
			return &NotYetLowerable{Reason: "the module already speaks " + nm + ", the name class " + info.name + "'s static initialization needs"}
		}
		taken[staticInitName(info)] = true
	}
	info.staticInit = append(info.staticInit, step)
	return nil
}

// thrownMethodDecls emits the two methods that let a thrown instance ride the
// runtime's panic path: ErrorName reports the source class name, the spelling
// a catch and the uncaught reporter print, and ErrorMessage reads the
// instance's message field. Together they satisfy value.Thrown, so
// value.Throw takes the instance as-is; a catch that recovers it binds a
// *value.Error carrying this name and message.
func (r *Renderer) thrownMethodDecls(info *classInfo) []ast.Decl {
	recv := func() *ast.FieldList {
		return &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ident(info.recv)},
			Type:  star(ident(info.goName)),
		}}}
	}
	stringResult := func() *ast.FieldList {
		return &ast.FieldList{List: []*ast.Field{{Type: ident("string")}}}
	}
	var msgField string
	for _, f := range info.fields {
		if f.prop == "message" {
			msgField = f.goName
		}
	}
	nameDecl := &ast.FuncDecl{
		Recv: recv(),
		Name: ident("ErrorName"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: stringResult()},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{
			Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(info.name)}},
		}}},
	}
	msgDecl := &ast.FuncDecl{
		Recv: recv(),
		Name: ident("ErrorMessage"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: stringResult()},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{
			Results: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{
				X:   &ast.SelectorExpr{X: ident(info.recv), Sel: ident(msgField)},
				Sel: ident("ToGoString"),
			}}},
		}}},
	}
	return []ast.Decl{nameDecl, msgDecl}
}

// staticVarDecl emits one static field as a package var with an explicit type,
// so the declaration reads the same as the struct field it sits next to. A field
// whose initializer is not a plain constant declares its var zero-valued here and
// runs its initializer as an assignment in the class's static init function, in
// member order, since package-level Go has no ordered execution.
func (r *Renderer) staticVarDecl(f classField) (ast.Decl, error) {
	goType, err := r.typeExpr(r.prog.TypeAt(f.ident))
	if err != nil {
		return nil, err
	}
	spec := &ast.ValueSpec{
		Names: []*ast.Ident{ident(f.goName)},
		Type:  goType,
	}
	if !f.runtimeInit && f.init != nil {
		rhs, err := r.lowerExpr(f.init)
		if err != nil {
			return nil, err
		}
		rhs, err = r.coerceToTarget(rhs, f.init, f.ident)
		if err != nil {
			return nil, err
		}
		spec.Values = []ast.Expr{rhs}
	}
	return &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{spec},
	}, nil
}

// classStruct emits the struct: one field per instance field, in declaration
// order, each carrying the source property name in a json tag the same way an
// object shape's struct does, so a reflection walk recovers the exact key.
func (r *Renderer) classStruct(info *classInfo) (ast.Decl, error) {
	fields := &ast.FieldList{}
	if len(info.vprops) > 0 {
		// The hierarchy root carries the vtable pointer as its first field, set
		// once by each constructor and read by every virtual entry. It is
		// unexported machinery, not a source property, so it takes no json tag
		// and the reflection walk in the value package skips it.
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident("vtable")},
			Type:  star(ident(vtableTypeName(info))),
		})
	}
	if info.base != nil {
		// The embedded base sits first, the way a hand-written derived struct
		// puts it: Go promotion serves the inherited fields and methods, and
		// encoding/json flattens the embedded fields, which keeps the wire shape
		// on the JS own-property layout (a base constructor assigns onto this, so
		// every inherited field is an own property of the instance).
		fields.List = append(fields.List, &ast.Field{Type: ident(info.base.goName)})
	}
	for _, f := range info.fields {
		goType, err := r.typeExpr(r.prog.TypeAt(f.ident))
		if err != nil {
			return nil, err
		}
		// A private field (#x) is invisible to JSON.stringify in JavaScript, so its
		// Go field takes the json:"-" tag that omits it rather than the property
		// name a public field carries.
		tag := f.prop
		if strings.HasPrefix(f.prop, "#") {
			tag = "-"
		}
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident(f.goName)},
			Type:  goType,
			Tag:   &ast.BasicLit{Kind: token.STRING, Value: "`json:\"" + tag + "\"`"},
		})
	}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{
			Name: ident(info.goName),
			Type: &ast.StructType{Fields: fields},
		}},
	}, nil
}

// classCtor emits the NewX constructor. JavaScript runs the field initializers
// in declaration order and then the constructor body, so the general form is an
// allocation, the initializer assignments, the lowered body with this bound to
// the receiver, and a return. When every initializer and every body statement
// is a pure this.field = value store, the whole sequence folds to the composite
// literal a Go programmer writes: return &Point{X: x, Y: y}. An unassigned
// non-optional field would read as undefined in JavaScript but as the Go zero
// value here; the strict checker the frontend always runs proves every field
// definitely assigned before a program reaches lowering, so the two never
// diverge on a checked program.
func (r *Renderer) classCtor(info *classInfo) ([]ast.Decl, error) {
	if info.ctor != nil && subtreeHasKind(r.prog, info.ctor, frontend.NodeReturnStatement) {
		return nil, &NotYetLowerable{Reason: "a return inside a constructor is a later slice"}
	}
	params, err := r.ctorParamFields(info)
	if err != nil {
		return nil, err
	}

	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = info, info.recv
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()

	if info.hasVTable() || info.chainHasAbstract() {
		// A virtual hierarchy splits construction: NewX allocates and pins the
		// class's vtable before any initializer runs, so a virtual call inside a
		// base constructor already dispatches to the derived override, the order
		// JavaScript runs (a JS instance is its final class from the first line
		// of the base constructor, unlike C++). An abstract chain takes the same
		// split even without a vtable, because an abstract base has no NewX to
		// fold or copy from, only its init to run on the embedded base in place.
		return r.splitCtorDecls(info, params)
	}
	body, err := r.ctorBody(info)
	if err != nil {
		return nil, err
	}
	return []ast.Decl{&ast.FuncDecl{
		Name: ident("New" + info.goName),
		Type: &ast.FuncType{
			Params:  params,
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(ident(info.goName))}}},
		},
		Body: body,
	}}, nil
}

// ctorParamFields builds the constructor's Go parameter list from the declared
// parameter nodes, shared by the NewX declaration and the initX split a virtual
// hierarchy adds. A static optional parameter hands back the same way paramFields
// makes a plain method's: its Go type is the value.Opt[T] the T | undefined union
// renders, which the constructor body reads as a bare T, so admitting it would
// emit Go that does not compile. The call-site defaulting that would fill an
// omitted slot is a later slice. A dynamic optional is fine, an omitted slot
// holds the undefined its value.Value already models.
func (r *Renderer) ctorParamFields(info *classInfo) (*ast.FieldList, error) {
	var sig frontend.Signature
	haveSig := false
	if info.ctor != nil {
		if s, ok := r.prog.SignatureAt(info.ctor); ok {
			sig, haveSig = s, true
		}
	}
	params := &ast.FieldList{}
	for i, p := range info.ctorParams {
		nameNode := r.paramNameNode(p)
		pname, ok := localName(r.prog.Text(nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "constructor parameter name is not a Go identifier"}
		}
		pt := r.prog.TypeAt(nameNode)
		if haveSig && i >= sig.MinArgs && pt.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return nil, &NotYetLowerable{Flags: pt.Flags, Reason: "optional parameter needs call-site defaulting, a later slice"}
		}
		goType, err := r.typeExpr(pt)
		if err != nil {
			return nil, err
		}
		params.List = append(params.List, &ast.Field{Names: []*ast.Ident{ident(pname)}, Type: goType})
	}
	return params, nil
}

// ctorBody builds the constructor's statements, folding to a composite literal
// when every store is pure. A return statement inside a constructor body needs
// its own lowering (a bare return must still yield the receiver), so it hands
// back for now.
func (r *Renderer) ctorBody(info *classInfo) (*ast.BlockStmt, error) {
	if lit, ok, err := r.ctorCompositeFold(info); err != nil {
		return nil, err
	} else if ok {
		return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{lit}}}}, nil
	}

	// The general form: allocate, construct the base value, run the field
	// initializers in order, run the body with this bound to the receiver,
	// return the receiver. The base assignment comes first because super()
	// runs before the derived field initializers, and the lowered body skips
	// the super statement it replaces.
	stmts := []ast.Stmt{&ast.AssignStmt{
		Lhs: []ast.Expr{ident(info.recv)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(info.goName)}}},
	}}
	if info.base != nil {
		// The this-free statements before super() run first, the order
		// JavaScript runs them, then the base assignment stands in for super().
		if len(info.preSuper) > 0 {
			pre, err := r.scopedBlockRange(r.ctorBlock(info), 0, len(info.preSuper))
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, pre.List...)
		}
		superVal, err := r.superCtorExpr(info)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ident(info.recv), Sel: ident(info.base.goName)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{superVal},
		})
	}
	inits, err := r.fieldInitStmts(info)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, inits...)
	body, err := r.ctorBodyStmts(info)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, body...)
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{ident(info.recv)}})
	return &ast.BlockStmt{List: stmts}, nil
}

// fieldInitStmts lowers the declared field initializers to the receiver-field
// assignments the general constructor form runs, in declaration order, the
// order JavaScript runs them.
func (r *Renderer) fieldInitStmts(info *classInfo) ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	for _, f := range info.fields {
		if f.init == nil {
			continue
		}
		rhs, err := r.lowerExpr(f.init)
		if err != nil {
			return nil, err
		}
		rhs, err = r.coerceToTarget(rhs, f.init, f.ident)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ident(info.recv), Sel: ident(f.goName)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{rhs},
		})
	}
	return stmts, nil
}

// ctorBodyStmts lowers the declared constructor's body with this bound to the
// receiver, skipping the validated super() statement a derived constructor
// leads with, which the caller has already emitted as the base construction.
func (r *Renderer) ctorBodyStmts(info *classInfo) ([]ast.Stmt, error) {
	if info.ctor == nil {
		return nil, nil
	}
	skip := 0
	if info.base != nil {
		// The pre-super statements and the validated super() call are all emitted
		// by the caller ahead of the base assignment; the body picks up after them.
		skip = len(info.preSuper) + 1
	}
	body, err := r.scopedBlock(r.ctorBlock(info), skip)
	if err != nil {
		return nil, err
	}
	return body.List, nil
}

// ctorBlock returns the declared constructor's body block.
func (r *Renderer) ctorBlock(info *classInfo) frontend.Node {
	var block frontend.Node
	for _, k := range r.prog.Children(info.ctor) {
		if k.Kind() == frontend.NodeBlock {
			block = k
		}
	}
	return block
}

// superCtorExpr builds the base value a derived constructor stores or folds:
// *NewBase(args), the dereferenced base constructor call. With a declared
// constructor the validated super arguments lower and coerce against the base
// constructor's parameters, the same bridging new Base(args) applies; with no
// constructor the synthesized one passes its own parameters (the base's)
// straight through by name.
func (r *Renderer) superCtorExpr(info *classInfo) (ast.Expr, error) {
	args, err := r.superCtorArgs(info)
	if err != nil {
		return nil, err
	}
	return &ast.StarExpr{X: &ast.CallExpr{Fun: ident("New" + info.base.goName), Args: args}}, nil
}

// superCtorArgs lowers the arguments a derived constructor hands its base:
// the validated super arguments coerced against the base constructor's
// parameters, or the synthesized pass-through of its own parameters by name
// when the class declares no constructor.
func (r *Renderer) superCtorArgs(info *classInfo) ([]ast.Expr, error) {
	base := info.base
	args := make([]ast.Expr, 0, len(info.ctorParams))
	if info.ctor == nil {
		for _, p := range info.ctorParams {
			pname, ok := localName(r.prog.Text(r.paramNameNode(p)))
			if !ok {
				return nil, &NotYetLowerable{Reason: "constructor parameter name is not a Go identifier"}
			}
			args = append(args, ident(pname))
		}
	} else {
		for i, a := range info.superArgs {
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			lowered, err = r.coerceToTarget(lowered, a, r.paramNameNode(base.ctorParams[i]))
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
	}
	return args, nil
}

// ctorCompositeFold recognizes the constructor whose whole effect is storing
// pure values into fields, the overwhelmingly common shape, and folds it to the
// one-line composite literal a person writes. Every field initializer and every
// body statement must be a this.field = expr store of a pure, this-free
// expression; the fold keeps the last store per field, which is sound exactly
// because a pure expression's evaluation cannot be observed out of order or
// skipped. Anything else reports ok=false and the general form runs.
func (r *Renderer) ctorCompositeFold(info *classInfo) (ast.Expr, bool, error) {
	values := map[string]frontend.Node{}
	for _, f := range info.fields {
		if f.init == nil {
			continue
		}
		if !r.pureCtorValue(f.init) {
			return nil, false, nil
		}
		values[f.prop] = f.init
	}
	if info.ctor != nil {
		var block frontend.Node
		for _, k := range r.prog.Children(info.ctor) {
			if k.Kind() == frontend.NodeBlock {
				block = k
			}
		}
		body := r.prog.Children(block)
		if info.base != nil {
			// The first statement is the validated super() call; it folds as the
			// base element below rather than as a field store.
			body = body[1:]
		}
		for _, stmt := range body {
			prop, rhs, ok := r.thisFieldStore(stmt)
			if !ok || !r.pureCtorValue(rhs) {
				return nil, false, nil
			}
			// Own fields only, never the chained lookup: a store into an inherited
			// field has no slot in the literal being built, so it must fail the
			// fold and take the general form, where Go promotion carries it.
			if _, isField := info.fieldByName(prop); !isField {
				return nil, false, nil
			}
			values[prop] = rhs
		}
	}

	lit := &ast.CompositeLit{Type: ident(info.goName)}
	if info.base != nil {
		// The base element evaluates first in the literal, and every other
		// element is pure, so the base constructor's effects keep the order
		// super() gives them even when it is not itself pure.
		superVal, err := r.superCtorExpr(info)
		if err != nil {
			return nil, false, err
		}
		lit.Elts = append(lit.Elts, &ast.KeyValueExpr{Key: ident(info.base.goName), Value: superVal})
	}
	for _, f := range info.fields {
		v, ok := values[f.prop]
		if !ok {
			continue
		}
		rhs, err := r.lowerExpr(v)
		if err != nil {
			return nil, false, err
		}
		rhs, err = r.coerceToTarget(rhs, v, f.ident)
		if err != nil {
			return nil, false, err
		}
		lit.Elts = append(lit.Elts, &ast.KeyValueExpr{Key: ident(f.goName), Value: rhs})
	}
	return &ast.UnaryExpr{Op: token.AND, X: lit}, true, nil
}

// thisFieldStore recognizes a statement of the exact shape this.f = expr and
// returns the field name and the value expression.
func (r *Renderer) thisFieldStore(stmt frontend.Node) (string, frontend.Node, bool) {
	if stmt.Kind() != frontend.NodeExpressionStatement {
		return "", nil, false
	}
	kids := r.prog.Children(stmt)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeBinaryExpression {
		return "", nil, false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return "", nil, false
	}
	if parts[0].Kind() != frontend.NodePropertyAccessExpression {
		return "", nil, false
	}
	tkids := r.prog.Children(parts[0])
	if len(tkids) != 2 || tkids[0].Kind() != frontend.NodeThisKeyword {
		return "", nil, false
	}
	return r.prog.Text(tkids[1]), parts[2], true
}

// pureCtorValue reports whether an expression may be reordered into the
// composite-literal fold: built only of names, literals, parentheses, and
// operators, with no call, no allocation, no assignment, and no this. A false
// answer is never wrong, it just keeps the general constructor form.
func (r *Renderer) pureCtorValue(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeIdentifier, frontend.NodeNumericLiteral, frontend.NodeStringLiteral,
		frontend.NodeBigIntLiteral, frontend.NodeNoSubstitutionTemplateLiteral,
		frontend.NodeTrueKeyword, frontend.NodeFalseKeyword:
		return true
	case frontend.NodeParenthesizedExpression, frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.pureCtorValue(kids[0])
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return false
		}
		if op := r.prog.Text(parts[1]); op == "=" {
			return false
		} else if _, compound := compoundBaseOp(op); compound {
			return false
		}
		return r.pureCtorValue(parts[0]) && r.pureCtorValue(parts[2])
	default:
		return false
	}
}

// classMethodDecl emits one method as a pointer-receiver Go method: the
// signature from the checker like a function declaration's, the body lowered
// with this bound to the receiver. The emitted name is passed in because a
// virtual method's body emits under its Impl name while everything else keeps
// the member's own Go name.
func (r *Renderer) classMethodDecl(info *classInfo, m classMethod, name string) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	if len(sig.TypeParams) != 0 && r.typeSubst == nil {
		// A generic method with no active substitution is one no specialization pass
		// entered: it has no single Go form and hands back. When a specialization is
		// being emitted, genericMethodDecls sets typeSubst so the bare type parameters
		// in the parameter and return types resolve to this instantiation's concrete
		// types, and the method lowers under its mangled name like a plain one.
		return nil, &NotYetLowerable{Reason: "generic method needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}
	// A method reuses the function parameter lowering so a default-valued parameter
	// becomes a plain Go field the method call fills at the call site, the same
	// call-site defaulting a top-level function takes. A default that reads an earlier
	// parameter, which the call site cannot reconstruct, hands back here since a method
	// has no callee-scope variadic fallback, so only a call-site-reconstructible default
	// lowers.
	// The class is in scope before the parameter and result types render, so a
	// method that takes or returns the polymorphic this type (a fluent `return
	// this`, the class-is-its-own-iterator [Symbol.iterator]) resolves it to the
	// receiver's own Go type rather than reaching the type-parameter handback.
	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = info, info.recv
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()

	params, err := r.funcParamFields(m.node, sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}

	prevRet := r.retType
	r.retType = sig.Return
	defer func() { r.retType = prevRet }()

	body, err := r.blockOf(m.node)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ident(info.recv)},
			Type:  star(ident(info.goName)),
		}}},
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// genericMethodDecls emits the specialized Go methods a generic method needs, one
// per distinct instantiation its call sites fixed (Wrap_num, Wrap_str). A Go method
// carries no type parameter, so this is the method analogue of funcDecls: the
// specialization set is the map collectMonoMethods filled before any body lowered,
// and each specialization lowers through classMethodDecl with its substitution
// active, so the bare type parameters in the signature and body resolve to that
// instantiation's concrete types. A generic method no call site monomorphizes, or
// one whose Go form does not model the vtable split yet, has no specialization to
// emit and hands back rather than leave a call site naming a method that was never
// emitted.
func (r *Renderer) genericMethodDecls(info *classInfo, m classMethod) ([]ast.Decl, error) {
	if m.generator || m.async {
		return nil, &NotYetLowerable{Reason: "a generic generator or async method is a later slice"}
	}
	if info.vprops[m.prop] || info.overrides[m.prop] {
		return nil, &NotYetLowerable{Reason: "a generic method in a class hierarchy is a later slice"}
	}
	specs := r.monoMethodSpecs[m.node]
	if len(specs) == 0 {
		return nil, &NotYetLowerable{Reason: "a generic method no call site monomorphizes is a later slice"}
	}
	out := make([]ast.Decl, 0, len(specs))
	for _, sp := range specs {
		prev := r.typeSubst
		r.typeSubst = sp.subst
		md, err := r.classMethodDecl(info, m, m.goName+"_"+sp.suffix)
		r.typeSubst = prev
		if err != nil {
			return nil, err
		}
		out = append(out, md)
	}
	return out, nil
}

// generatorMethodDecl emits a generator method as a Go method that returns a running
// coroutine, the same *value.Gen[Y] a top-level generator function returns
// (generator.go): the method's one statement hands the caller value.NewGen wrapping
// the body, and each yield in that body suspends the coroutine until the consumer
// pulls again. The receiver is in scope for the body, so a this inside it reads the
// method's receiver, captured by the goroutine closure. Two iterations do not share
// state because each call mints a fresh *value.Gen.
func (r *Renderer) generatorMethodDecl(info *classInfo, m classMethod) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "generator method has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic method needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}
	params, err := r.paramFields(sig)
	if err != nil {
		return nil, err
	}

	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = info, info.recv
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()

	yieldGo, newGen, err := r.generatorCoroutine(m.node)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ident(info.recv)},
			Type:  star(ident(info.goName)),
		}}},
		Name: ident(m.goName),
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{{Type: star(index(sel("value", "Gen"), yieldGo))}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{newGen}}}},
	}, nil
}

// staticFuncDecl emits one static method as a package function. The body
// lowers with no current class and no this name, so a this inside a static
// body hands back the way it does in a plain function; the class's statics
// stay reachable because A.total routes through the class name, not this.
func (r *Renderer) staticFuncDecl(m classMethod) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "static method has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic method needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}
	// A static method takes the same call-site defaulting a top-level function does,
	// so a default-valued parameter lowers to a plain Go field the call fills; a
	// default that reads an earlier parameter hands back here.
	params, err := r.funcParamFields(m.node, sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}

	prevRet := r.retType
	r.retType = sig.Return
	defer func() { r.retType = prevRet }()
	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = nil, ""
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()

	body, err := r.blockOf(m.node)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Name: ident(m.goName),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// asyncMethodDecl emits an await-free async instance method as a synchronous method
// returning a settled value.Promise. An async body runs synchronously up to its
// first await, and an await-free body runs to completion on the calling stack, so
// the body lowers unchanged inside a func the runtime runs now: value.Async turns a
// normal return into a resolved promise and a thrown value into a rejected one,
// matching the JavaScript rule that a synchronous throw inside an async body becomes
// a rejection. The method's Go result is the *value.Promise the declared Promise<T>
// return maps to; the inner func returns the element type T the body's returns
// carry.
func (r *Renderer) asyncMethodDecl(info *classInfo, m classMethod, name string) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic method needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}
	params, err := r.paramFields(sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = info, info.recv
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()
	body, err := r.asyncBody(sig.Return, m.node)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ident(info.recv)},
			Type:  star(ident(info.goName)),
		}}},
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// asyncStaticFuncDecl is asyncMethodDecl for a static async method, a package
// function with no receiver returning a settled promise.
func (r *Renderer) asyncStaticFuncDecl(m classMethod) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "static method has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic method needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}
	params, err := r.paramFields(sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = nil, ""
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()
	body, err := r.asyncBody(sig.Return, m.node)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Name: ident(m.goName),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// asyncBody lowers an async method's body into the single return statement that
// mints its settled promise: return value.Async(func() T { <body> }) for a valued
// promise, or value.AsyncVoid(func() { <body> }) for a Promise<void>. The body
// lowers with the element type T as its return type, so each `return expr` in the
// source coerces expr to T and returns from the inner func, which value.Async then
// settles. ret is the method's declared Promise<T> return; retNode is the method
// node whose block holds the body.
func (r *Renderer) asyncBody(ret frontend.Type, retNode frontend.Node) (*ast.BlockStmt, error) {
	// A body that awaits suspends at each await and cannot run to completion on the
	// calling stack, so it lowers through the coroutine rather than the synchronous
	// value.Async wrapping this function builds for an await-free body.
	if r.bodyHasAwait(retNode) {
		return r.asyncCoroutineBody(ret, retNode)
	}
	elem, ok := r.promiseElem(ret)
	if !ok {
		return nil, &NotYetLowerable{Reason: "an async method whose return is not a Promise is a later slice"}
	}
	prevRet := r.retType
	if isVoidReturn(elem) {
		r.retType = frontend.Type{}
	} else {
		r.retType = elem
	}
	defer func() { r.retType = prevRet }()
	inner, err := r.blockOf(retNode)
	if err != nil {
		return nil, err
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	if isVoidReturn(elem) {
		lit := &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: inner}
		call := &ast.CallExpr{Fun: sel("value", "AsyncVoid"), Args: []ast.Expr{lit}}
		return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{call}}}}, nil
	}
	et, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: et}}}},
		Body: inner,
	}
	call := &ast.CallExpr{Fun: sel("value", "Async"), Args: []ast.Expr{lit}}
	return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{call}}}}, nil
}

// newClass lowers new Point(args) to the NewPoint constructor call, coercing
// each argument against the declared parameter the way an assignment coerces.
// The argument count must match the constructor exactly; optional and default
// parameters are the same later slice they are for functions.
func (r *Renderer) newClass(info *classInfo, argNodes []frontend.Node) (ast.Expr, error) {
	// The checker rejects new on an abstract class; this guard keeps a unit
	// that somehow carries one from referencing the NewX that does not exist.
	if info.abstract {
		return nil, &NotYetLowerable{Reason: "constructing the abstract class " + info.name + " is not lowerable"}
	}
	if len(argNodes) > len(info.ctorParams) {
		return nil, &NotYetLowerable{Reason: "new " + info.name + " with an argument count that differs from the constructor is a later slice"}
	}
	args := make([]ast.Expr, 0, len(info.ctorParams))
	for i, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		paramName := r.paramNameNode(info.ctorParams[i])
		lowered, err = r.coerceToTarget(lowered, a, paramName)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	// A trailing parameter the call omits is an optional the checker accepted the
	// short call against. A dynamic optional's slot fills with value.Undefined,
	// the absent value the language binds; a static omission has no Go value to
	// stand in and hands back.
	for i := len(argNodes); i < len(info.ctorParams); i++ {
		if !r.isDynamic(r.paramNameNode(info.ctorParams[i])) {
			return nil, &NotYetLowerable{Reason: "new " + info.name + " omitting a non-dynamic optional argument is a later slice"}
		}
		r.requireImport(valuePkg)
		args = append(args, sel("value", "Undefined"))
	}
	return &ast.CallExpr{Fun: ident("New" + info.goName), Args: args}, nil
}

// classMethodCall lowers recv.method(args) on a class instance to the Go method
// call, bridging each argument against its declared parameter the way an
// assignment does. A super.m() call on a virtual method routes to the Impl
// body directly: the entry would dispatch through the instance's own vtable
// and recurse into the very override that is calling super, while Go promotion
// on the embedded base picks the nearest ancestor's Impl, the method JS super
// names.
func (r *Renderer) classMethodCall(info *classInfo, recv ast.Expr, method string, argNodes []frontend.Node, viaSuper bool) (ast.Expr, error) {
	m, ok := info.lookupMethod(method)
	if !ok {
		if _, isField := info.lookupField(method); isField {
			return nil, &NotYetLowerable{Reason: "calling a field of class " + info.name + " as a function is a later slice"}
		}
		return nil, &NotYetLowerable{Reason: "class " + info.name + " has no method ." + method + " this slice lowers"}
	}
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	if len(argNodes) > len(sig.Params) {
		return nil, &NotYetLowerable{Reason: "method call with an argument count that differs from the declaration is a later slice"}
	}
	name := m.goName
	if len(sig.TypeParams) != 0 {
		// A generic method resolves to the mangled Go method the call's type arguments
		// fix (Wrap_num for wrap(5)), the one genericMethodDecls emitted for this
		// instantiation. A call whose type arguments do not mangle names no emitted
		// method, so it hands back rather than call a Go method that does not exist.
		spec, ok := r.methodMonoSpec(sig, argNodes)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a generic method call whose type arguments do not monomorphize is a later slice"}
		}
		name = m.goName + "_" + spec.suffix
	}
	if viaSuper && info.isVirtual(method) {
		name = implName(m)
	}
	args := make([]ast.Expr, 0, len(sig.Params))
	paramNodes := r.funcParamNodes(m.node)
	for i, a := range argNodes {
		a = r.argForDefaultedSlot(paramNodes, sig, i, a)
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		lowered, err = r.bridgeArg(lowered, a, sig.Params[i].Type)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	if err := r.fillOmittedMethodArgs(&args, m.node, sig, len(argNodes)); err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}, Args: args}, nil
}

// staticMethodCall lowers A.m(args) to the package function the static method
// became. Arguments lower plainly, the same way an instance method call's do.
func (r *Renderer) staticMethodCall(info *classInfo, method string, argNodes []frontend.Node) (ast.Expr, error) {
	m, ok := info.staticMethodByName(method)
	if !ok {
		if _, isField := info.staticByName(method); isField {
			return nil, &NotYetLowerable{Reason: "calling static ." + method + " of class " + info.name + " as a function is a later slice"}
		}
		return nil, &NotYetLowerable{Reason: "class " + info.name + " has no static method ." + method + " this slice lowers"}
	}
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "static method has no call signature"}
	}
	if len(argNodes) > len(sig.Params) {
		return nil, &NotYetLowerable{Reason: "method call with an argument count that differs from the declaration is a later slice"}
	}
	args := make([]ast.Expr, 0, len(sig.Params))
	paramNodes := r.funcParamNodes(m.node)
	for i, a := range argNodes {
		a = r.argForDefaultedSlot(paramNodes, sig, i, a)
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		lowered, err = r.bridgeArg(lowered, a, sig.Params[i].Type)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	if err := r.fillOmittedMethodArgs(&args, m.node, sig, len(argNodes)); err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: ident(m.goName), Args: args}, nil
}

// argForDefaultedSlot returns the expression to lower for argument i of a method
// call with these parameter nodes. An explicit undefined argument in a slot whose
// parameter carries a call-site-reconstructible default counts as a missing argument,
// so the parameter's default stands in for it, matching the language rule that
// undefined triggers a default exactly as an omission does. Any other argument, and
// an undefined in a slot with no default or a default that reads an earlier
// parameter, passes through unchanged.
func (r *Renderer) argForDefaultedSlot(paramNodes []frontend.Node, sig frontend.Signature, i int, a frontend.Node) frontend.Node {
	if !r.isUndefinedLiteral(a) {
		return a
	}
	if def, ok := r.paramDefaultNode(paramNodes, i); ok && !r.defaultReadsOwnParam(sig, def) {
		return def
	}
	return a
}

// fillOmittedMethodArgs fills the trailing argument slots a short method call left
// empty, the same call-site defaulting a top-level function takes. A slot whose
// parameter carries a default fills with that default, evaluated at the call site
// where the class binding is visible and bridged to the parameter type; a default
// that reads an earlier parameter needs the callee's scope and hands back, matching
// the funcParamFields gate the method declaration already applied. A slot with no
// default fills with value.Undefined only when its type is dynamic, the absent value
// the language binds; a static optional with no default has no Go value to stand in
// and hands back.
func (r *Renderer) fillOmittedMethodArgs(args *[]ast.Expr, node frontend.Node, sig frontend.Signature, from int) error {
	paramNodes := r.funcParamNodes(node)
	for i := from; i < len(sig.Params); i++ {
		if def, ok := r.paramDefaultNode(paramNodes, i); ok {
			if r.defaultReadsOwnParam(sig, def) {
				return &NotYetLowerable{Reason: "a method default that reads an earlier parameter needs the callee's scope, a later slice"}
			}
			lowered, err := r.lowerExpr(def)
			if err != nil {
				return err
			}
			lowered, err = r.bridgeArg(lowered, def, sig.Params[i].Type)
			if err != nil {
				return err
			}
			*args = append(*args, lowered)
			continue
		}
		if sig.Params[i].Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return &NotYetLowerable{Reason: "a call omitting a non-dynamic optional argument is a later slice"}
		}
		r.requireImport(valuePkg)
		*args = append(*args, sel("value", "Undefined"))
	}
	return nil
}

// classFieldOfTarget resolves a property access to the class field it stores
// into: a static field when the receiver is the class name, an instance field
// when the receiver is this inside a lowered class body or a plain identifier
// the checker types as a registered class instance. The receiver restriction
// is what makes a compound store and an increment sound: a name or this cannot
// carry a side effect, so lowering it twice (the read and the write) cannot
// double one. A recognized class receiver with an unrecognized property, or a
// property only an accessor serves, is an error, not a fall-through, so the
// store never silently routes to a lowering that would treat the instance as
// something else.
func (r *Renderer) classFieldOfTarget(target frontend.Node) (*classInfo, classField, bool, error) {
	tkids := r.prog.Children(target)
	if len(tkids) != 2 {
		return nil, classField{}, false, nil
	}
	obj := tkids[0]
	if obj.Kind() != frontend.NodeThisKeyword && obj.Kind() != frontend.NodeIdentifier {
		return nil, classField{}, false, nil
	}
	prop := r.prog.Text(tkids[1])
	if obj.Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(obj); ok {
			f, ok := info.staticByName(prop)
			if !ok {
				return nil, classField{}, false, &NotYetLowerable{Reason: "storing into static ." + prop + " of class " + info.name + " is a later slice"}
			}
			return info, f, true, nil
		}
	}
	info, ok := r.classReceiver(obj)
	if !ok {
		return nil, classField{}, false, nil
	}
	f, ok := info.lookupField(prop)
	if !ok {
		if _, isSet := info.lookupSetter(prop); isSet {
			return nil, classField{}, false, &NotYetLowerable{Reason: "a compound store or increment through the ." + prop + " accessor of class " + info.name + " is a later slice"}
		}
		if _, isGet := info.lookupGetter(prop); isGet {
			return nil, classField{}, false, &NotYetLowerable{Reason: "storing into the read-only accessor ." + prop + " of class " + info.name + " is a later slice"}
		}
		return nil, classField{}, false, &NotYetLowerable{Reason: "storing into ." + prop + " of class " + info.name + " is a later slice"}
	}
	return info, f, true, nil
}

// classFieldTarget resolves a property access to the lowered field selector
// when it is a class field store target, for the increment statement that needs
// only the left-hand side.
func (r *Renderer) classFieldTarget(target frontend.Node) (ast.Expr, bool, error) {
	_, _, ok, err := r.classFieldOfTarget(target)
	if err != nil || !ok {
		return nil, ok, err
	}
	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	return lhs, true, nil
}

// classFieldAssign lowers a store into a class member: this.x = v inside a
// class body or p.x = v on an instance local becomes the Go field assignment,
// A.x = v on a static becomes the package var assignment, and a plain store
// through a set accessor becomes the Set call. It reports ok=false when the
// statement is not a store into a recognized class member, so lowerUpdate
// falls through to the local-identifier assignment. The receiver must be this
// or a plain identifier, so a compound store (p.x += v), which reads the
// receiver twice, cannot double a side effect.
func (r *Renderer) classFieldAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	opText := r.prog.Text(parts[1])
	baseOp, compound := compoundBaseOp(opText)
	if opText != "=" && !compound {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodePropertyAccessExpression {
		return nil, false, nil
	}
	if stmt, ok, err := r.classSetterStore(target, opText, parts[2]); ok || err != nil {
		return stmt, ok, err
	}
	if stmt, ok, err := r.staticSetterStore(target, opText, parts[2]); ok || err != nil {
		return stmt, ok, err
	}
	_, f, ok, err := r.classFieldOfTarget(target)
	if err != nil || !ok {
		return nil, ok, err
	}

	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	var rhs ast.Expr
	if compound {
		rhs, err = r.combineBinary(baseOp, target, parts[2])
		if err != nil {
			return nil, false, err
		}
		if r.combineIsDynamic(baseOp, target, parts[2]) && !r.isDynamic(target) {
			rhs, err = r.coerceDynamicToStatic(rhs, target)
			if err != nil {
				return nil, false, err
			}
		}
	} else {
		rhs, err = r.lowerExpr(parts[2])
		if err != nil {
			return nil, false, err
		}
		rhs, err = r.coerceToTarget(rhs, parts[2], f.ident)
		if err != nil {
			return nil, false, err
		}
	}

	// A self-referential store collapses to the compound form and a step of one to
	// ++/--, so p.count = p.count + 1 prints p.count++ and A.total = A.total + 1
	// prints aTotal++, the way lowerAssign collapses a local's step.
	tok := token.ASSIGN
	if b, ok := rhs.(*ast.BinaryExpr); ok {
		if ctok, ok := compoundAssignToken(b.Op); ok && sameStoreTarget(b.X, lhs) {
			tok = ctok
			rhs = b.Y
		}
	}
	assign := &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: tok, Rhs: []ast.Expr{rhs}}
	if inc, ok := incDecFromStep(assign); ok {
		return inc, true, nil
	}
	return assign, true, nil
}

// classSetterStore lowers a plain assignment through a set accessor, a.x = v,
// to the ExprStmt a.SetX(v), coercing the value against the setter's declared
// parameter. It declines a compound store (classFieldOfTarget names that
// hand-back) and a receiver that is the class name, which a same-named static
// owns.
func (r *Renderer) classSetterStore(target frontend.Node, opText string, valueNode frontend.Node) (ast.Stmt, bool, error) {
	tkids := r.prog.Children(target)
	if len(tkids) != 2 {
		return nil, false, nil
	}
	obj := tkids[0]
	if obj.Kind() != frontend.NodeThisKeyword && obj.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	if obj.Kind() == frontend.NodeIdentifier {
		if _, isName := r.classNameRef(obj); isName {
			return nil, false, nil
		}
	}
	info, ok := r.classReceiver(obj)
	if !ok {
		return nil, false, nil
	}
	s, ok := info.lookupSetter(r.prog.Text(tkids[1]))
	if !ok || opText != "=" {
		return nil, false, nil
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, false, err
	}
	rhs, err := r.lowerExpr(valueNode)
	if err != nil {
		return nil, false, err
	}
	rhs, err = r.coerceToTarget(rhs, valueNode, s.param)
	if err != nil {
		return nil, false, err
	}
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(s.goName)}, Args: []ast.Expr{rhs}}
	return &ast.ExprStmt{X: call}, true, nil
}

// staticSetterStore lowers a plain assignment through a static set accessor,
// C.x = v, to the ExprStmt CSetX(v), the static twin of classSetterStore. The
// receiver is the class name, and the value coerces against the setter's
// declared parameter. A compound store or increment through the accessor reads
// the property back, which a write-only accessor cannot serve, so it hands back
// with its own reason rather than falling through to the static-field path.
func (r *Renderer) staticSetterStore(target frontend.Node, opText string, valueNode frontend.Node) (ast.Stmt, bool, error) {
	tkids := r.prog.Children(target)
	if len(tkids) != 2 || tkids[0].Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	info, ok := r.classNameRef(tkids[0])
	if !ok {
		return nil, false, nil
	}
	s, ok := info.staticSetterByName(r.prog.Text(tkids[1]))
	if !ok {
		return nil, false, nil
	}
	if opText != "=" {
		return nil, false, &NotYetLowerable{Reason: "a compound store or increment through the static ." + s.prop + " accessor of class " + info.name + " is a later slice"}
	}
	rhs, err := r.lowerExpr(valueNode)
	if err != nil {
		return nil, false, err
	}
	rhs, err = r.coerceToTarget(rhs, valueNode, s.param)
	if err != nil {
		return nil, false, err
	}
	call := &ast.CallExpr{Fun: ident(s.goName), Args: []ast.Expr{rhs}}
	return &ast.ExprStmt{X: call}, true, nil
}

// sameStoreTarget reports whether two expressions are the same simple store
// target, a bare identifier (a static's package var) or an identifier dot
// field, so the compound collapse only fires on the exact target the
// assignment writes.
func sameStoreTarget(a, b ast.Expr) bool {
	switch x := a.(type) {
	case *ast.Ident:
		y, ok := b.(*ast.Ident)
		return ok && x.Name == y.Name
	case *ast.SelectorExpr:
		y, ok := b.(*ast.SelectorExpr)
		if !ok || x.Sel.Name != y.Sel.Name {
			return false
		}
		xi, ok1 := x.X.(*ast.Ident)
		yi, ok2 := y.X.(*ast.Ident)
		return ok1 && ok2 && xi.Name == yi.Name
	}
	return false
}

// subtreeHasKind reports whether any node under n (inclusive) has the kind.
func subtreeHasKind(prog *frontend.Program, n frontend.Node, kind frontend.NodeKind) bool {
	if n.Kind() == kind {
		return true
	}
	for _, c := range prog.Children(n) {
		if subtreeHasKind(prog, c, kind) {
			return true
		}
	}
	return false
}

// firstWord returns the first whitespace-delimited word of s, for the modifier
// checks that read a member's source spelling.
func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i]
	}
	return s
}
