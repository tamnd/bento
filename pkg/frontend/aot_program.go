package frontend

import (
	"strings"
	"sync"

	"github.com/tamnd/bento/pkg/frontend/adapter"
)

// Program is bento's typed-program handle. It wraps an adapter.TSAdapter and an
// opaque program handle, and turns the adapter's handle-returning calls into the
// value-returning queries the partitioner and lowering consume. The interners
// map opaque type and symbol handles to small integer ids, so a bento Type or
// Symbol carries an id, never a typescript-go object, and the program looks the
// handle back up when a follow-up structural query comes in.
type Program struct {
	adapter adapter.TSAdapter
	handle  adapter.ProgramHandle

	types   *interner[adapter.TypeHandle]
	symbols *interner[adapter.SymbolHandle]
}

type typeID int
type symbolID int

// interner assigns a stable small id to each distinct handle and maps back.
// Handles compare by interface identity, which for the adapter's pointer-backed
// handles is pointer identity, exactly what we want. The mutex makes intern and
// lookup safe under concurrent queries, which the partitioner relies on because
// Pass A classifies units in parallel (06_compile_vs_interpret.md section 4.2).
type interner[H comparable] struct {
	mu    sync.Mutex
	ids   map[H]int
	items []H
}

func newInterner[H comparable]() *interner[H] {
	return &interner[H]{ids: map[H]int{}}
}

func (in *interner[H]) intern(h H) int {
	in.mu.Lock()
	defer in.mu.Unlock()
	if id, ok := in.ids[h]; ok {
		return id
	}
	id := len(in.items)
	in.items = append(in.items, h)
	in.ids[h] = id
	return id
}

func (in *interner[H]) lookup(id int) H {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.items[id]
}

// Wrap builds a Program over a given adapter and program handle. Load uses it
// with the real adapter, the only implementation; taking an interface keeps the
// typescript-go coupling behind the adapter package rather than leaking it here.
// It is the single constructor for a Program so the interners are always
// initialized.
func Wrap(a adapter.TSAdapter, h adapter.ProgramHandle) *Program {
	return &Program{
		adapter: a,
		handle:  h,
		types:   newInterner[adapter.TypeHandle](),
		symbols: newInterner[adapter.SymbolHandle](),
	}
}

// nodeRef is the concrete Node the frontend hands out. It carries the opaque
// handle and a back pointer to the program so File resolves the file kind.
type nodeRef struct {
	h adapter.NodeHandle
	p *Program
}

func (n nodeRef) Kind() NodeKind { return n.p.adapter.KindOf(n.h) }

func (n nodeRef) Pos() Pos {
	start, _, _ := n.p.adapter.SpanOf(n.h)
	return Pos(start)
}

func (n nodeRef) End() Pos {
	_, end, _ := n.p.adapter.SpanOf(n.h)
	return Pos(end)
}

func (n nodeRef) File() SourceFile {
	_, _, path := n.p.adapter.SpanOf(n.h)
	return SourceFile{Path: path, Kind: n.p.adapter.FileKindOf(path)}
}

func (p *Program) wrapNode(h adapter.NodeHandle) Node { return nodeRef{h: h, p: p} }

func (p *Program) unwrapNode(n Node) adapter.NodeHandle { return n.(nodeRef).h }

// wrapType turns an opaque handle into a bento Type, reading the coarse flags
// eagerly and stashing the handle behind an id for later structural queries. A
// nil handle (a query with no answer) becomes the zero Type.
func (p *Program) wrapType(h adapter.TypeHandle) Type {
	if h == nil {
		return Type{}
	}
	id := p.types.intern(h)
	return Type{Flags: p.adapter.TypeFlagsOf(p.handle, h), id: typeID(id)}
}

func (p *Program) typeHandle(t Type) adapter.TypeHandle { return p.types.lookup(int(t.id)) }

func (p *Program) wrapSymbol(h adapter.SymbolHandle) Symbol {
	id := p.symbols.intern(h)
	return Symbol{
		Name:  p.adapter.SymbolName(h),
		Flags: p.adapter.SymbolFlagsOf(h),
		id:    symbolID(id),
	}
}

func (p *Program) symbolHandle(s Symbol) adapter.SymbolHandle { return p.symbols.lookup(int(s.id)) }

func (p *Program) wrapSignature(si adapter.SignatureInfo) Signature {
	sig := Signature{
		Return:  p.wrapType(si.Return),
		MinArgs: si.MinArgs,
	}
	for _, param := range si.Params {
		sig.Params = append(sig.Params, p.wrapParam(param))
	}
	for _, tp := range si.TypeParams {
		sig.TypeParams = append(sig.TypeParams, p.wrapType(tp))
	}
	if si.RestParam != nil {
		rest := p.wrapParam(*si.RestParam)
		sig.RestParam = &rest
	}
	return sig
}

func (p *Program) wrapParam(pi adapter.ParamInfo) Param {
	return Param{Name: pi.Name, Type: p.wrapType(pi.Type), Optional: pi.Optional}
}

// SourceFiles returns the top-level node of each file, the roots a consumer
// walks to enumerate declarations. The synthetic ambient library bento injects
// (its Node global declarations) is filtered out here, so it is invisible to
// every consumer the way lib.d.ts is: it supplies globals to the checker but is
// not a file the program lists, lowers, or reports on.
func (p *Program) SourceFiles() []Node {
	handles := p.adapter.SourceFiles(p.handle)
	out := make([]Node, 0, len(handles))
	for _, h := range handles {
		n := p.wrapNode(h)
		if n.File().Path == ambientPath {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Children returns the direct child nodes of a node, so a consumer can walk a
// body without ever touching a typescript-go node.
func (p *Program) Children(n Node) []Node {
	handles := p.adapter.ChildrenOf(p.unwrapNode(n))
	out := make([]Node, len(handles))
	for i, h := range handles {
		out[i] = p.wrapNode(h)
	}
	return out
}

// ForClauses returns a for statement's initializer, condition, incrementor, and
// body by role, with a flag for each of the three optional header clauses. It
// reads roles straight off the node, so an omitted clause is reported as absent
// rather than silently collapsing onto another role the way a bare child walk
// would. The node must be a for statement.
func (p *Program) ForClauses(n Node) ForClauses {
	i, c, r, b := p.adapter.ForClausesOf(p.unwrapNode(n))
	out := ForClauses{Body: p.wrapNode(b)}
	if i != nil {
		out.Init, out.HasInit = p.wrapNode(i), true
	}
	if c != nil {
		out.Cond, out.HasCond = p.wrapNode(c), true
	}
	if r != nil {
		out.Incr, out.HasIncr = p.wrapNode(r), true
	}
	return out
}

// Text returns n's own source text with no leading trivia, the identifier name,
// literal, or operator token lowering emits into the generated Go.
func (p *Program) Text(n Node) string {
	raw := p.adapter.TextOf(p.unwrapNode(n))
	// An identifier's source spelling may carry unicode escapes, so the same
	// name can be declared one way and read another; lowering mangles on the
	// name it denotes, not its spelling, so both forms have to decode to one
	// string before mangling. Only identifiers take escapes this way, and only
	// a spelling with a backslash needs the work, so the escape-free identifier
	// and every operator, keyword, and literal token skip it.
	if strings.IndexByte(raw, '\\') >= 0 && n.Kind() == NodeIdentifier {
		return decodeIdentEscapes(raw)
	}
	return raw
}

// TypeAt returns the type of the expression at n, with flow narrowing applied at
// n's exact position. This is the query the partitioner runs on every parameter,
// local, and return, and the query lowering runs on every expression.
func (p *Program) TypeAt(n Node) Type {
	return p.wrapType(p.adapter.TypeOfNode(p.handle, p.unwrapNode(n)))
}

// DeclaredTypeAt returns the un-narrowed declared type of the variable used at
// n alongside the narrowed type from TypeAt, so the partitioner can tell when a
// branch relies on narrowing.
func (p *Program) DeclaredTypeAt(n Node) (declared, narrowed Type, ok bool) {
	dh, ok := p.adapter.DeclaredTypeOfNode(p.handle, p.unwrapNode(n))
	if !ok {
		return Type{}, Type{}, false
	}
	return p.wrapType(dh), p.TypeAt(n), true
}

// TypeOfSymbol returns the declared type of a symbol at its declaration.
func (p *Program) TypeOfSymbol(s Symbol) Type {
	return p.wrapType(p.adapter.TypeOfSymbol(p.handle, p.symbolHandle(s)))
}

// Widen returns the widened type of a literal type, turning the literal 42 into
// number, which the partitioner uses when a literal escapes its context.
func (p *Program) Widen(t Type) Type {
	return p.wrapType(p.adapter.WidenType(p.handle, p.typeHandle(t)))
}

// SymbolAt returns the symbol a name node resolves to, and ok=false for a node
// that resolves to no symbol.
func (p *Program) SymbolAt(n Node) (Symbol, bool) {
	h, ok := p.adapter.SymbolOfNode(p.handle, p.unwrapNode(n))
	if !ok {
		return Symbol{}, false
	}
	return p.wrapSymbol(h), true
}

// ShorthandValueSymbolAt returns the local binding an object-literal shorthand
// member reads, so the use walk can credit `{ x }` to the outer `x` rather than to
// the property the shorthand declares. It reports false for a node that is not a
// shorthand member.
func (p *Program) ShorthandValueSymbolAt(n Node) (Symbol, bool) {
	h, ok := p.adapter.ShorthandValueSymbolOfNode(p.handle, p.unwrapNode(n))
	if !ok {
		return Symbol{}, false
	}
	return p.wrapSymbol(h), true
}

// TypeSymbol returns the symbol a type was declared by, and ok=false for an
// anonymous type. Lowering uses it to walk from a class instance type back to
// the class declaration that names it.
func (p *Program) TypeSymbol(t Type) (Symbol, bool) {
	h, ok := p.adapter.SymbolOfType(p.handle, p.typeHandle(t))
	if !ok {
		return Symbol{}, false
	}
	return p.wrapSymbol(h), true
}

// Aliased follows an import or export alias to the symbol it ultimately names.
func (p *Program) Aliased(s Symbol) Symbol {
	return p.wrapSymbol(p.adapter.AliasedSymbol(p.handle, p.symbolHandle(s)))
}

// Declarations returns the declaration nodes for a symbol; a symbol can have
// several, for overloads or merged interfaces.
func (p *Program) Declarations(s Symbol) []Node {
	handles := p.adapter.DeclarationsOf(p.handle, p.symbolHandle(s))
	out := make([]Node, len(handles))
	for i, h := range handles {
		out[i] = p.wrapNode(h)
	}
	return out
}

// SignatureAt returns the signature the checker resolved for a specific call,
// new expression, or function declaration node, after overload resolution.
func (p *Program) SignatureAt(n Node) (Signature, bool) {
	si, ok := p.adapter.ResolvedSignature(p.handle, p.unwrapNode(n))
	if !ok {
		return Signature{}, false
	}
	return p.wrapSignature(si), true
}

// Signatures returns the call signatures and construct signatures of a function
// type separately, because lowering emits a Go func for the former and a
// constructor for the latter.
func (p *Program) Signatures(t Type) (call, construct []Signature) {
	callInfos, ctorInfos := p.adapter.SignaturesOf(p.handle, p.typeHandle(t))
	for _, si := range callInfos {
		call = append(call, p.wrapSignature(si))
	}
	for _, si := range ctorInfos {
		construct = append(construct, p.wrapSignature(si))
	}
	return call, construct
}

// UnionMembers returns the constituent types of a union, or a single-element
// slice for a non-union.
func (p *Program) UnionMembers(t Type) []Type {
	handles := p.adapter.UnionOf(p.handle, p.typeHandle(t))
	out := make([]Type, len(handles))
	for i, h := range handles {
		out[i] = p.wrapType(h)
	}
	return out
}

// IntersectionMembers returns the constituent types of an intersection, or a
// single-element slice for a non-intersection. It is how the lowerer sees through a
// branded alias (number & { __brand }) to the underlying primitive a go: defined
// type projects to (section 6.11).
func (p *Program) IntersectionMembers(t Type) []Type {
	handles := p.adapter.IntersectionOf(p.handle, p.typeHandle(t))
	out := make([]Type, len(handles))
	for i, h := range handles {
		out[i] = p.wrapType(h)
	}
	return out
}

// Properties returns the named members of an object type, each with its own type
// and optionality.
func (p *Program) Properties(t Type) []Property {
	infos := p.adapter.PropertiesOf(p.handle, p.typeHandle(t))
	out := make([]Property, len(infos))
	for i, pi := range infos {
		out[i] = Property{
			Name:              pi.Name,
			Type:              p.wrapType(pi.Type),
			Optional:          pi.Optional,
			Readonly:          pi.Readonly,
			WriteType:         p.wrapType(pi.WriteType),
			DivergentAccessor: pi.DivergentAccessor,
		}
	}
	return out
}

// ElementType returns the element type of an array or tuple type, and ok=false
// for a non-array.
func (p *Program) ElementType(t Type) (Type, bool) {
	h, ok := p.adapter.ElementOf(p.handle, p.typeHandle(t))
	if !ok {
		return Type{}, false
	}
	return p.wrapType(h), true
}

// TypeArguments returns the instantiated type arguments of a generic type, the [T]
// in a Generator<T> or a Map<K, V>. It reads the element type off a built-in
// generic whose members the structural queries do not expand, so lowering can spell
// the Go representation the generic maps to. A non-generic type returns no
// arguments.
func (p *Program) TypeArguments(t Type) []Type {
	hs := p.adapter.TypeArgsOf(p.handle, p.typeHandle(t))
	if len(hs) == 0 {
		return nil
	}
	out := make([]Type, 0, len(hs))
	for _, h := range hs {
		out = append(out, p.wrapType(h))
	}
	return out
}

// TupleElements returns the positional elements of a tuple type, the [K, V] in a
// [string, number], and false for a non-tuple. Each element carries its element
// type, whether the position is optional or the rest tail, and its label, so
// lowering can spell the positional struct the tuple maps to. This is the query
// that separates a tuple from the array ElementType reports on: an array has one
// element type for every index, a tuple has a fixed sequence of them.
func (p *Program) TupleElements(t Type) ([]TupleElem, bool) {
	elems, ok := p.adapter.TupleElemsOf(p.handle, p.typeHandle(t))
	if !ok {
		return nil, false
	}
	out := make([]TupleElem, len(elems))
	for i, e := range elems {
		out[i] = TupleElem{
			Type:     p.wrapType(e.Type),
			Optional: e.Optional,
			Rest:     e.Rest,
			Label:    e.Label,
		}
	}
	return out, true
}

// LiteralValue returns the literal value of a literal type, so lowering can fold
// closed unions into integer tags and refine integers.
func (p *Program) LiteralValue(t Type) (LiteralValue, bool) {
	return p.adapter.LiteralOf(p.handle, p.typeHandle(t))
}

// Imports returns the resolved imports of a source file, each mapping a written
// specifier to the file it resolved to and the kind of import it is.
func (p *Program) Imports(f SourceFile) []ResolvedImport {
	infos := p.adapter.ImportsOf(p.handle, f.Path)
	out := make([]ResolvedImport, len(infos))
	for i, ri := range infos {
		out[i] = ResolvedImport{
			Specifier: ri.Specifier,
			Resolved:  SourceFile{Path: ri.ResolvedFile, Kind: p.adapter.FileKindOf(ri.ResolvedFile)},
			Kind:      ri.Kind,
		}
	}
	return out
}

// Diagnostics returns all syntactic and semantic diagnostics for the program,
// passed through from the checker so bento output matches tsc.
func (p *Program) Diagnostics() []Diagnostic {
	infos := p.adapter.Diagnostics(p.handle)
	out := make([]Diagnostic, 0, len(infos))
	for _, di := range infos {
		out = append(out, p.wrapDiagnostic(di))
	}
	return out
}

func (p *Program) wrapDiagnostic(di adapter.DiagnosticInfo) Diagnostic {
	d := Diagnostic{
		Code:     di.Code,
		Category: di.Category,
		Message:  di.Message,
		Span:     Span{Start: Pos(di.Start), End: Pos(di.End)},
	}
	if di.File != "" {
		f := SourceFile{Path: di.File, Kind: p.adapter.FileKindOf(di.File)}
		d.File = &f
	}
	for _, r := range di.Related {
		d.Related = append(d.Related, p.wrapDiagnostic(r))
	}
	return d
}

// LineColumn resolves a byte offset in a file to a Position.
func (p *Program) LineColumn(f SourceFile, at Pos) Position {
	line, col := p.adapter.LineColumnOf(p.handle, f.Path, int(at))
	return Position{Line: line, Column: col}
}

// Revision reports the pinned typescript-go revision the backing adapter was
// built against, empty for the fake or a not-yet-pinned real adapter.
func (p *Program) Revision() string { return p.adapter.Revision() }
