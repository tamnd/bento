package adapter

// FakeAdapter is a working in-memory implementation of TSAdapter, built by hand
// rather than by a real checker. It exists so the partitioner and lowering, and
// their tests, have a typed program to run against while the real
// typescript-go adapter is blocked upstream (see adapter.go). It is not a mock:
// every query answers from a real graph of nodes, types, symbols, and
// signatures the test assembled, so a pass exercised against it exercises the
// same code paths it will run against the real adapter.
//
// Build a program with the Fake builder:
//
//	f := adapter.NewFake()
//	num := f.Prim(TypeNumber)
//	fn := f.Func("area", f.Sig([]ParamInfo{{Name: "r", Type: num}}, num))
//	a, prog := f.Program(fn)
//
// then hand (a, prog) to frontend.Wrap to get a *frontend.Program.
type FakeAdapter struct {
	revision string
}

// fakeProgram is the FakeAdapter's ProgramHandle: the whole graph of roots plus
// the module import edges and diagnostics a test attached.
type fakeProgram struct {
	roots       []*FakeNode
	imports     map[string][]ResolvedImportInfo
	diagnostics []DiagnosticInfo
}

func (*fakeProgram) programHandle() {}

// FakeNode is a hand-built AST node. It is exported so tests can assemble a body
// and assert against it; the marker method keeps it a valid NodeHandle.
type FakeNode struct {
	Kind     NodeKind
	Children []*FakeNode
	Type     *FakeType
	Symbol   *FakeSymbol
	Sig      *SignatureInfo // set on function and call nodes
	Start    int
	End      int
	FilePath string
}

func (*FakeNode) nodeHandle() {}

// FakeType is a hand-built type. Only the fields relevant to its Flags are read.
type FakeType struct {
	Flags     TypeFlags
	Union     []*FakeType
	Props     []PropertyInfo
	Elem      *FakeType
	Literal   *LiteralValue
	CallSigs  []SignatureInfo
	CtorSigs  []SignatureInfo
	Declared  *FakeType // un-narrowed declared type, when this is a narrowed view
	WidenedTo *FakeType // result of WidenType, defaults to self
}

func (*FakeType) typeHandle() {}

// FakeSymbol is a hand-built bound name.
type FakeSymbol struct {
	NameText string
	Flags    SymbolFlags
	Decls    []*FakeNode
	Type     *FakeType
	AliasOf  *FakeSymbol
}

func (*FakeSymbol) symbolHandle() {}

// Fake is the builder that assembles a FakeAdapter program. Its methods return
// the exported node and type structs so a test can wire relationships directly.
type Fake struct {
	prog *fakeProgram
}

// NewFake returns an empty builder.
func NewFake() *Fake {
	return &Fake{prog: &fakeProgram{imports: map[string][]ResolvedImportInfo{}}}
}

// Prim builds a primitive type from one or more flag bits.
func (f *Fake) Prim(flags TypeFlags) *FakeType { return &FakeType{Flags: flags} }

// Any is the untyped top, a convenience for the common blocker case.
func (f *Fake) Any() *FakeType { return &FakeType{Flags: TypeAny} }

// Object builds an object type from its properties.
func (f *Fake) Object(props ...PropertyInfo) *FakeType {
	return &FakeType{Flags: TypeObject, Props: props}
}

// Union builds a union type from its members.
func (f *Fake) Union(members ...*FakeType) *FakeType {
	return &FakeType{Flags: TypeUnion, Union: members}
}

// Array builds an array type over an element type.
func (f *Fake) Array(elem *FakeType) *FakeType {
	return &FakeType{Flags: TypeObject, Elem: elem}
}

// StringLit builds a string-literal type, the checker's view of a type whose one
// value is v. It carries both TypeString and TypeLiteral like the real checker,
// and a LiteralValue so LiteralOf answers, which is what the closed-union enum
// lowering reads.
func (f *Fake) StringLit(v string) *FakeType {
	return &FakeType{Flags: TypeString | TypeLiteral, Literal: &LiteralValue{Kind: LiteralString, Str: v}}
}

// Prop is a small helper to build a PropertyInfo.
func (f *Fake) Prop(name string, t *FakeType) PropertyInfo {
	return PropertyInfo{Name: name, Type: t}
}

// Sig builds a call signature.
func (f *Fake) Sig(params []ParamInfo, ret *FakeType) *SignatureInfo {
	min := 0
	for _, p := range params {
		if !p.Optional {
			min++
		}
	}
	return &SignatureInfo{Params: params, Return: ret, MinArgs: min}
}

// Param is a small helper to build a ParamInfo.
func (f *Fake) Param(name string, t *FakeType) ParamInfo {
	return ParamInfo{Name: name, Type: t}
}

// Func builds a function declaration node with a resolved signature and body.
func (f *Fake) Func(name string, sig *SignatureInfo, body ...*FakeNode) *FakeNode {
	return &FakeNode{
		Kind:     NodeFunctionDeclaration,
		Sig:      sig,
		Children: body,
		Symbol:   &FakeSymbol{NameText: name, Flags: SymbolFunction},
	}
}

// Node builds a generic node of a kind with an attached type and children, for
// assembling the uses inside a function body.
func (f *Fake) Node(kind NodeKind, t *FakeType, children ...*FakeNode) *FakeNode {
	return &FakeNode{Kind: kind, Type: t, Children: children}
}

// Import records a resolved import edge for a file, feeding ImportsOf.
func (f *Fake) Import(file string, edge ResolvedImportInfo) {
	f.prog.imports[file] = append(f.prog.imports[file], edge)
}

// Diagnostic records a diagnostic the program reports.
func (f *Fake) Diagnostic(d DiagnosticInfo) {
	f.prog.diagnostics = append(f.prog.diagnostics, d)
}

// Program finalizes the builder with a set of root nodes and returns an adapter
// plus the program handle ready for frontend.Wrap.
func (f *Fake) Program(roots ...*FakeNode) (TSAdapter, ProgramHandle) {
	f.prog.roots = roots
	return &FakeAdapter{revision: "fake"}, f.prog
}

func typeOf(t *FakeType) TypeHandle {
	if t == nil {
		return nil
	}
	return t
}

func asType(h TypeHandle) *FakeType {
	if h == nil {
		return nil
	}
	return h.(*FakeType)
}

func asNode(h NodeHandle) *FakeNode { return h.(*FakeNode) }
func asSymbol(h SymbolHandle) *FakeSymbol {
	if h == nil {
		return nil
	}
	return h.(*FakeSymbol)
}

func (a *FakeAdapter) BuildProgram([]string, CompilerOptions, Host) (ProgramHandle, error) {
	return &fakeProgram{imports: map[string][]ResolvedImportInfo{}}, nil
}

func (a *FakeAdapter) UpdateProgram(prev ProgramHandle, _ []FileChange) (ProgramHandle, error) {
	return prev, nil
}

func (a *FakeAdapter) TypeOfNode(_ ProgramHandle, n NodeHandle) TypeHandle {
	return typeOf(asNode(n).Type)
}

func (a *FakeAdapter) TypeOfSymbol(_ ProgramHandle, s SymbolHandle) TypeHandle {
	return typeOf(asSymbol(s).Type)
}

func (a *FakeAdapter) WidenType(_ ProgramHandle, t TypeHandle) TypeHandle {
	ft := asType(t)
	if ft != nil && ft.WidenedTo != nil {
		return ft.WidenedTo
	}
	return t
}

func (a *FakeAdapter) DeclaredTypeOfNode(_ ProgramHandle, n NodeHandle) (TypeHandle, bool) {
	ft := asNode(n).Type
	if ft != nil && ft.Declared != nil {
		return ft.Declared, true
	}
	if ft == nil {
		return nil, false
	}
	return ft, true
}

func (a *FakeAdapter) SymbolOfNode(_ ProgramHandle, n NodeHandle) (SymbolHandle, bool) {
	s := asNode(n).Symbol
	if s == nil {
		return nil, false
	}
	return s, true
}

func (a *FakeAdapter) AliasedSymbol(_ ProgramHandle, s SymbolHandle) SymbolHandle {
	sym := asSymbol(s)
	for sym != nil && sym.AliasOf != nil {
		sym = sym.AliasOf
	}
	return sym
}

func (a *FakeAdapter) DeclarationsOf(_ ProgramHandle, s SymbolHandle) []NodeHandle {
	out := []NodeHandle{}
	for _, d := range asSymbol(s).Decls {
		out = append(out, d)
	}
	return out
}

func (a *FakeAdapter) SymbolName(s SymbolHandle) string { return asSymbol(s).NameText }

func (a *FakeAdapter) SymbolFlagsOf(s SymbolHandle) SymbolFlags { return asSymbol(s).Flags }

func (a *FakeAdapter) ResolvedSignature(_ ProgramHandle, call NodeHandle) (SignatureInfo, bool) {
	sig := asNode(call).Sig
	if sig == nil {
		return SignatureInfo{}, false
	}
	return *sig, true
}

func (a *FakeAdapter) SignaturesOf(_ ProgramHandle, t TypeHandle) (call, construct []SignatureInfo) {
	ft := asType(t)
	if ft == nil {
		return nil, nil
	}
	return ft.CallSigs, ft.CtorSigs
}

func (a *FakeAdapter) TypeFlagsOf(_ ProgramHandle, t TypeHandle) TypeFlags {
	ft := asType(t)
	if ft == nil {
		return 0
	}
	return ft.Flags
}

func (a *FakeAdapter) UnionOf(_ ProgramHandle, t TypeHandle) []TypeHandle {
	ft := asType(t)
	if ft == nil || len(ft.Union) == 0 {
		return []TypeHandle{t}
	}
	out := make([]TypeHandle, len(ft.Union))
	for i, m := range ft.Union {
		out[i] = m
	}
	return out
}

func (a *FakeAdapter) PropertiesOf(_ ProgramHandle, t TypeHandle) []PropertyInfo {
	ft := asType(t)
	if ft == nil {
		return nil
	}
	return ft.Props
}

func (a *FakeAdapter) ElementOf(_ ProgramHandle, t TypeHandle) (TypeHandle, bool) {
	ft := asType(t)
	if ft == nil || ft.Elem == nil {
		return nil, false
	}
	return ft.Elem, true
}

func (a *FakeAdapter) LiteralOf(_ ProgramHandle, t TypeHandle) (LiteralValue, bool) {
	ft := asType(t)
	if ft == nil || ft.Literal == nil {
		return LiteralValue{}, false
	}
	return *ft.Literal, true
}

func (a *FakeAdapter) ImportsOf(p ProgramHandle, file string) []ResolvedImportInfo {
	return p.(*fakeProgram).imports[file]
}

func (a *FakeAdapter) Diagnostics(p ProgramHandle) []DiagnosticInfo {
	return p.(*fakeProgram).diagnostics
}

func (a *FakeAdapter) LineColumnOf(ProgramHandle, string, int) (int, int) { return 1, 0 }

func (a *FakeAdapter) SourceFiles(p ProgramHandle) []NodeHandle {
	roots := p.(*fakeProgram).roots
	out := make([]NodeHandle, len(roots))
	for i, r := range roots {
		out[i] = r
	}
	return out
}

func (a *FakeAdapter) KindOf(n NodeHandle) NodeKind { return asNode(n).Kind }

func (a *FakeAdapter) ChildrenOf(n NodeHandle) []NodeHandle {
	kids := asNode(n).Children
	out := make([]NodeHandle, len(kids))
	for i, k := range kids {
		out[i] = k
	}
	return out
}

func (a *FakeAdapter) SpanOf(n NodeHandle) (int, int, string) {
	fn := asNode(n)
	return fn.Start, fn.End, fn.FilePath
}

func (a *FakeAdapter) FileKindOf(string) FileKind { return FileTS }

func (a *FakeAdapter) Revision() string { return a.revision }
