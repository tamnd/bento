package adapter

// RealAdapter is the TSAdapter backed by the real TypeScript compiler. It drives
// microsoft/typescript-go through the public shim the bento fork adds (see
// pkg/frontend/adapter/version.go and the shim package), so every type, symbol,
// and signature bento reads is the checker's own answer, never a stand-in. This
// file is the single place in bento that imports typescript-go; the import-graph
// test in boundary_test.go keeps it that way.
//
// The adapter's opaque handles wrap the shim's pointers. Because the frontend
// interns TypeHandle and SymbolHandle by identity, the wrappers are value structs
// over a pointer, so two wraps of the same checker object compare equal and share
// one interned id.

import (
	"strings"
	"sync"

	"github.com/microsoft/typescript-go/shim"
)

// RealAdapter implements TSAdapter over the real checker.
type RealAdapter struct{}

// NewReal returns a real, typescript-go-backed adapter.
func NewReal() *RealAdapter { return &RealAdapter{} }

// realProgram is the RealAdapter's ProgramHandle: the compiled shim program, the
// set of input files bento supplied (so lib files are filtered out of the file
// listing), and the resolved import edges discovered during the build.
//
// The checker typescript-go hands back is not safe for concurrent use: it fills
// its caches lazily on demand, so two goroutines inside it at once race on those
// maps. The partitioner's Pass A classifies units in parallel, so every checker
// access goes through mu, serializing the checker while leaving the parallel walk
// of the immutable AST (ChildrenOf, SourceFiles) lock-free.
type realProgram struct {
	mu      sync.Mutex
	prog    *shim.Program
	inputs  map[string]bool
	order   []string
	imports map[string][]ResolvedImportInfo
	files   map[string]*shim.SourceFile
}

func (*realProgram) programHandle() {}

// checker locks the program and returns its checker; the caller must call the
// returned release when done, which unlocks. It is the single gate every
// checker-touching adapter method passes through.
func (rp *realProgram) checker() (*shim.Checker, func()) {
	rp.mu.Lock()
	return rp.prog.Checker(), rp.mu.Unlock
}

// rNode, rType, and rSymbol wrap the shim's pointers as opaque handles. The value
// receiver on the marker method means the struct value satisfies the interface,
// and the struct compares by the wrapped pointer, which is the identity the
// interner keys on.
type rNode struct{ n *shim.Node }
type rType struct{ t *shim.Type }
type rSymbol struct{ s *shim.Symbol }

func (rNode) nodeHandle()     {}
func (rType) typeHandle()     {}
func (rSymbol) symbolHandle() {}

func wrapType(t *shim.Type) TypeHandle {
	if t == nil {
		return nil
	}
	return rType{t: t}
}

func wrapSymbol(s *shim.Symbol) SymbolHandle {
	if s == nil {
		return nil
	}
	return rSymbol{s: s}
}

func nodeOf(h NodeHandle) *shim.Node { return h.(rNode).n }
func typeOfHandle(h TypeHandle) *shim.Type {
	if h == nil {
		return nil
	}
	return h.(rType).t
}
func symbolOf(h SymbolHandle) *shim.Symbol {
	if h == nil {
		return nil
	}
	return h.(rSymbol).s
}

func prog(p ProgramHandle) *realProgram { return p.(*realProgram) }

// BuildProgram compiles the roots and every file reachable from them into one
// checked program. It discovers the file set by parsing what it has, asking
// bento's own resolver (through the host) where each import points, reading that
// file, and repeating until the set stops growing. typescript-go then does its
// own resolution and checking over the complete in-memory file set, so the type
// view and bento's module graph describe the same files.
func (a *RealAdapter) BuildProgram(roots []string, opts CompilerOptions, host Host) (ProgramHandle, error) {
	files := map[string]string{}
	for _, r := range roots {
		if content, ok := host.ReadFile(r); ok {
			files[r] = content
		}
	}

	inputs := map[string]bool{}
	for name := range files {
		inputs[name] = true
	}

	shimRoots := append([]string(nil), roots...)
	shimOpts := shim.Options{
		RootFiles:            shimRoots,
		Loose:                !opts.Strict,
		Target:               opts.Target,
		ImportHelpers:        opts.ImportHelpers,
		AllowUnreachableCode: opts.AllowUnreachableCode,
	}

	for {
		p := shim.Compile(files, shimOpts)
		imports := map[string][]ResolvedImportInfo{}
		added := false
		for _, sf := range p.SourceFiles() {
			name := sf.FileName()
			if !inputs[name] {
				continue
			}
			for _, spec := range shim.ImportSpecifiers(sf) {
				resolved, kind, ok := host.ResolveModule(spec, name)
				imports[name] = append(imports[name], ResolvedImportInfo{
					Specifier:    spec,
					ResolvedFile: resolved,
					Kind:         kind,
				})
				if ok && resolved != "" && !inputs[resolved] {
					if content, has := host.ReadFile(resolved); has {
						files[resolved] = content
						inputs[resolved] = true
						added = true
						// A go: import's declarations are an ambient module block that
						// nothing resolves to by path, so it registers only once its
						// file is a parsed root. Promote it so the next pass sees the
						// declare module and binds the import.
						if kind == ImportGo {
							shimRoots = append(shimRoots, resolved)
							shimOpts.RootFiles = shimRoots
						}
					}
				}
			}
		}
		if !added {
			return finalizeProgram(p, inputs, imports), nil
		}
		p.Close()
	}
}

// finalizeProgram captures the input file set, their source files, and the
// import edges the last build pass resolved into a realProgram, keeping a stable
// input order for listing.
func finalizeProgram(p *shim.Program, inputs map[string]bool, imports map[string][]ResolvedImportInfo) *realProgram {
	rp := &realProgram{
		prog:    p,
		inputs:  inputs,
		imports: imports,
		files:   map[string]*shim.SourceFile{},
	}
	for _, sf := range p.SourceFiles() {
		name := sf.FileName()
		if !inputs[name] {
			continue
		}
		rp.files[name] = sf
		rp.order = append(rp.order, name)
	}
	return rp
}

// UpdateProgram rebuilds from scratch for now; incremental reuse is a later
// slice. It is correct, just not yet fast.
func (a *RealAdapter) UpdateProgram(prev ProgramHandle, _ []FileChange) (ProgramHandle, error) {
	return prev, nil
}

// TypeOfNode returns the type at a node, or nil when the node holds no value
// type. The checker answers a non-value position (a statement, a block, a source
// file) with its internal error type, which carries the Any flag; returning that
// verbatim would make every statement look like an untyped value. Mapping the
// error type to nil gives bento the zero Type there, matching the contract that
// TypeAt names the type of an expression and nothing else. A genuine any-typed
// expression returns the distinct any type and still reports Any.
func (a *RealAdapter) TypeOfNode(p ProgramHandle, n NodeHandle) TypeHandle {
	c, release := prog(p).checker()
	defer release()
	t := c.GetTypeAtLocation(nodeOf(n))
	if t == nil || t == c.GetErrorType() {
		return nil
	}
	return wrapType(t)
}

func (a *RealAdapter) TypeOfSymbol(p ProgramHandle, s SymbolHandle) TypeHandle {
	c, release := prog(p).checker()
	defer release()
	return wrapType(c.GetTypeOfSymbol(symbolOf(s)))
}

func (a *RealAdapter) WidenType(p ProgramHandle, t TypeHandle) TypeHandle {
	c, release := prog(p).checker()
	defer release()
	return wrapType(c.GetWidenedType(typeOfHandle(t)))
}

// DeclaredTypeOfNode returns the un-narrowed declared type of the variable used
// at n, which is the type of the symbol the name resolves to, distinct from the
// flow-narrowed type at that exact position.
func (a *RealAdapter) DeclaredTypeOfNode(p ProgramHandle, n NodeHandle) (TypeHandle, bool) {
	c, release := prog(p).checker()
	defer release()
	node := nodeOf(n)
	sym := c.GetSymbolAtLocation(node)
	if sym == nil {
		// A binding declaration node (the name in let [x] or let { x }) is not a
		// reference the checker resolves by location, so it falls back to the symbol
		// bound to the declaration, the same fallback SymbolOfNode takes. This yields
		// the binding's declared type off its annotation rather than the type narrowed
		// by the initializer at that position.
		sym = node.Symbol()
	}
	if sym == nil {
		return nil, false
	}
	return wrapType(c.GetTypeOfSymbol(sym)), true
}

// SymbolOfNode returns the symbol a node resolves to. For a reference the checker
// resolves it by location; for a declaration node, whose location the checker
// does not treat as a reference, it falls back to the symbol binding attached the
// declaration, so asking for the symbol of a function or class declaration yields
// that declaration's own symbol rather than nothing.
func (a *RealAdapter) SymbolOfNode(p ProgramHandle, n NodeHandle) (SymbolHandle, bool) {
	c, release := prog(p).checker()
	defer release()
	node := nodeOf(n)
	sym := c.GetSymbolAtLocation(node)
	if sym == nil {
		sym = node.Symbol()
	}
	if sym == nil {
		return nil, false
	}
	return wrapSymbol(sym), true
}

// ShorthandValueSymbolOfNode returns the local binding an object-literal shorthand
// member copies from. The checker resolves a `{ x }` member's identifier to the
// property it declares, so lowering asks here for the value symbol instead, the
// outer `x` the shorthand reads. It reports false for a node the checker does not
// treat as a shorthand assignment.
func (a *RealAdapter) ShorthandValueSymbolOfNode(p ProgramHandle, n NodeHandle) (SymbolHandle, bool) {
	c, release := prog(p).checker()
	defer release()
	sym := c.GetShorthandAssignmentValueSymbol(nodeOf(n))
	if sym == nil {
		return nil, false
	}
	return wrapSymbol(sym), true
}

// SymbolOfType returns the symbol a type was declared by, the checker field
// that links a class instance type back to its class declaration. It is a plain
// field read like DeclarationsOf, so it takes no checker lock. An anonymous
// type carries no symbol and reports false.
func (a *RealAdapter) SymbolOfType(_ ProgramHandle, t TypeHandle) (SymbolHandle, bool) {
	sym := typeOfHandle(t).Symbol()
	if sym == nil {
		return nil, false
	}
	return wrapSymbol(sym), true
}

func (a *RealAdapter) AliasedSymbol(p ProgramHandle, s SymbolHandle) SymbolHandle {
	c, release := prog(p).checker()
	defer release()
	return wrapSymbol(c.SkipAlias(symbolOf(s)))
}

func (a *RealAdapter) DeclarationsOf(_ ProgramHandle, s SymbolHandle) []NodeHandle {
	sym := symbolOf(s)
	out := make([]NodeHandle, 0, len(sym.Declarations))
	for _, d := range sym.Declarations {
		out = append(out, rNode{n: d})
	}
	return out
}

func (a *RealAdapter) SymbolName(s SymbolHandle) string { return symbolOf(s).Name }

func (a *RealAdapter) SymbolFlagsOf(s SymbolHandle) SymbolFlags {
	return mapSymbolFlags(symbolOf(s).Flags)
}

// ResolvedSignature returns the signature bento reads at a node. The checker
// answers this two different ways depending on the node: for a call or new
// expression the signature is the one overload resolution picked, obtained with
// GetResolvedSignature; for a function-like declaration it is the declared
// signature, obtained with GetSignatureFromDeclaration. GetResolvedSignature
// panics on a declaration node, so the kind must steer the call.
func (a *RealAdapter) ResolvedSignature(p ProgramHandle, call NodeHandle) (SignatureInfo, bool) {
	c, release := prog(p).checker()
	defer release()
	node := nodeOf(call)
	var sig *shim.Signature
	switch node.Kind {
	case shim.KindCallExpression, shim.KindNewExpression:
		sig = c.GetResolvedSignature(node)
	default:
		if isFunctionLikeKind(node.Kind) {
			sig = c.GetSignatureFromDeclaration(node)
		}
	}
	if sig == nil {
		return SignatureInfo{}, false
	}
	return sigInfo(c, sig), true
}

// isFunctionLikeKind reports whether a node kind is a function-like declaration
// that carries its own signature, the set ResolvedSignature reads a declared
// signature from rather than resolving an overload.
func isFunctionLikeKind(k shim.Kind) bool {
	switch k {
	case shim.KindFunctionDeclaration,
		shim.KindFunctionExpression,
		shim.KindArrowFunction,
		shim.KindMethodDeclaration,
		shim.KindGetAccessor,
		shim.KindSetAccessor,
		shim.KindConstructor:
		return true
	default:
		return false
	}
}

func (a *RealAdapter) SignaturesOf(p ProgramHandle, t TypeHandle) (call, construct []SignatureInfo) {
	c, release := prog(p).checker()
	defer release()
	ty := typeOfHandle(t)
	for _, sig := range c.GetSignaturesOfType(ty, shim.SignatureKindCall) {
		call = append(call, sigInfo(c, sig))
	}
	for _, sig := range c.GetSignaturesOfType(ty, shim.SignatureKindConstruct) {
		construct = append(construct, sigInfo(c, sig))
	}
	return call, construct
}

// sigInfo turns a checker signature into the adapter's SignatureInfo, reading
// each parameter's type and optionality from its symbol and the return type from
// the signature. A rest parameter is reported separately from the fixed ones.
func sigInfo(c *shim.Checker, sig *shim.Signature) SignatureInfo {
	params := sig.Parameters()
	info := SignatureInfo{
		Return:  wrapType(c.GetReturnTypeOfSignature(sig)),
		MinArgs: sig.MinArgumentCount(),
	}
	for _, tp := range sig.TypeParameters() {
		info.TypeParams = append(info.TypeParams, wrapType(tp))
	}
	rest := sig.HasRestParameter()
	for i, ps := range params {
		pi := ParamInfo{
			Name:     ps.Name,
			Type:     wrapType(c.GetTypeOfSymbol(ps)),
			Optional: ps.Flags&shim.SymbolFlagsOptional != 0,
		}
		if rest && i == len(params)-1 {
			r := pi
			info.RestParam = &r
			continue
		}
		info.Params = append(info.Params, pi)
	}
	return info
}

func (a *RealAdapter) TypeFlagsOf(p ProgramHandle, t TypeHandle) TypeFlags {
	ty := typeOfHandle(t)
	if ty == nil {
		return 0
	}
	_, release := prog(p).checker()
	defer release()
	return mapTypeFlags(ty.Flags())
}

func (a *RealAdapter) UnionOf(p ProgramHandle, t TypeHandle) []TypeHandle {
	ty := typeOfHandle(t)
	if ty == nil {
		return []TypeHandle{t}
	}
	_, release := prog(p).checker()
	defer release()
	if ty.Flags()&shim.TypeFlagsUnion == 0 {
		return []TypeHandle{t}
	}
	members := ty.Types()
	out := make([]TypeHandle, 0, len(members))
	for _, m := range members {
		out = append(out, wrapType(m))
	}
	return out
}

func (a *RealAdapter) IntersectionOf(p ProgramHandle, t TypeHandle) []TypeHandle {
	ty := typeOfHandle(t)
	if ty == nil {
		return []TypeHandle{t}
	}
	_, release := prog(p).checker()
	defer release()
	if ty.Flags()&shim.TypeFlagsIntersection == 0 {
		return []TypeHandle{t}
	}
	members := ty.Types()
	out := make([]TypeHandle, 0, len(members))
	for _, m := range members {
		out = append(out, wrapType(m))
	}
	return out
}

func (a *RealAdapter) PropertiesOf(p ProgramHandle, t TypeHandle) []PropertyInfo {
	c, release := prog(p).checker()
	defer release()
	props := c.GetPropertiesOfType(typeOfHandle(t))
	out := make([]PropertyInfo, 0, len(props))
	for _, sym := range props {
		readT := c.GetTypeOfSymbol(sym)
		writeT := c.GetWriteTypeOfSymbol(sym)
		out = append(out, PropertyInfo{
			Name:              sym.Name,
			Type:              wrapType(readT),
			Optional:          sym.Flags&shim.SymbolFlagsOptional != 0,
			WriteType:         wrapType(writeT),
			DivergentAccessor: writeT != nil && writeT != readT,
		})
	}
	return out
}

func (a *RealAdapter) ElementOf(p ProgramHandle, t TypeHandle) (TypeHandle, bool) {
	c, release := prog(p).checker()
	defer release()
	ty := typeOfHandle(t)
	if ty == nil || !c.IsArrayType(ty) {
		return nil, false
	}
	elem := c.GetElementTypeOfArrayType(ty)
	if elem == nil {
		return nil, false
	}
	return wrapType(elem), true
}

// TypeArgsOf returns the instantiated type arguments of a generic type reference,
// the [T] in a Generator<T> or a Map<K, V>, so lowering can read the element type
// off a built-in generic the structural queries do not expand. GetTypeArguments
// assumes a type reference; a non-reference type reaches the checker's unhandled
// case and panics, so a recover converts that into no arguments and the caller
// keeps whatever handback it had rather than crashing the worker.
func (a *RealAdapter) TypeArgsOf(p ProgramHandle, t TypeHandle) (args []TypeHandle) {
	c, release := prog(p).checker()
	defer release()
	ty := typeOfHandle(t)
	if ty == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			args = nil
		}
	}()
	for _, arg := range c.GetTypeArguments(ty) {
		if arg == nil {
			return nil
		}
		args = append(args, wrapType(arg))
	}
	return args
}

// TupleElemsOf returns the positional elements of a tuple type, the [K, V] in a
// [string, number], so lowering can spell the positional struct the tuple maps
// to. It reports false for a non-tuple. The checker's TupleElementsOf gates on
// the tuple kind itself, reads the element types off the tuple reference's type
// arguments, and the optional flag, rest flag, and label off the tuple target, so
// this method only has to wrap each element type into a handle.
func (a *RealAdapter) TupleElemsOf(p ProgramHandle, t TypeHandle) ([]TupleElem, bool) {
	c, release := prog(p).checker()
	defer release()
	ty := typeOfHandle(t)
	if ty == nil {
		return nil, false
	}
	elems, ok := c.TupleElementsOf(ty)
	if !ok {
		return nil, false
	}
	out := make([]TupleElem, len(elems))
	for i, e := range elems {
		if e.Type == nil {
			return nil, false
		}
		out[i] = TupleElem{
			Type:     wrapType(e.Type),
			Optional: e.Optional,
			Rest:     e.Rest,
			Label:    e.Label,
		}
	}
	return out, true
}

// StringIndexOf returns the value type of an object type's string index signature,
// the string in { [x: string]: string }, and false for a type with no string
// indexer. It reads the checker's own index infos and keeps the one keyed by string,
// so a dictionary shape is told from a fixed one by the signature the checker
// resolved rather than by a name convention.
func (a *RealAdapter) StringIndexOf(p ProgramHandle, t TypeHandle) (TypeHandle, bool) {
	c, release := prog(p).checker()
	defer release()
	ty := typeOfHandle(t)
	if ty == nil {
		return nil, false
	}
	for _, info := range c.GetIndexInfosOfType(ty) {
		if info == nil {
			continue
		}
		key := info.KeyType()
		val := info.ValueType()
		if key == nil || val == nil {
			continue
		}
		if key.Flags()&shim.TypeFlagsString != 0 {
			return wrapType(val), true
		}
	}
	return nil, false
}

// NumberIndexOf returns the value type of a type's numeric index signature, the
// element union the checker computes for an array or tuple viewed as an array. It
// reads the checker's numeric index type, which is defined for a tuple (the union
// of its positions) where GetElementTypeOfArrayType is not, so a tuple borrowing an
// array method can be materialized as a value.Array over this element type.
func (a *RealAdapter) NumberIndexOf(p ProgramHandle, t TypeHandle) (TypeHandle, bool) {
	c, release := prog(p).checker()
	defer release()
	ty := typeOfHandle(t)
	if ty == nil {
		return nil, false
	}
	elem := c.GetNumberIndexType(ty)
	if elem == nil {
		return nil, false
	}
	return wrapType(elem), true
}

func (a *RealAdapter) LiteralOf(p ProgramHandle, t TypeHandle) (LiteralValue, bool) {
	ty := typeOfHandle(t)
	if ty == nil {
		return LiteralValue{}, false
	}
	_, release := prog(p).checker()
	defer release()
	if ty.Flags()&shim.TypeFlagsLiteral == 0 {
		return LiteralValue{}, false
	}
	switch v := shim.LiteralValue(ty).(type) {
	case string:
		return LiteralValue{Kind: LiteralString, Str: v}, true
	case bool:
		return LiteralValue{Kind: LiteralBoolean, Bool: v}, true
	default:
		// Number and bigint literals carry checker-internal value types that
		// need a shim-side accessor to cross the boundary without loss; until
		// that lands, they hand back so lowering routes the unit to the engine.
		return LiteralValue{}, false
	}
}

func (a *RealAdapter) ImportsOf(p ProgramHandle, file string) []ResolvedImportInfo {
	return prog(p).imports[file]
}

func (a *RealAdapter) Diagnostics(p ProgramHandle) []DiagnosticInfo {
	rp := prog(p)
	rp.mu.Lock()
	defer rp.mu.Unlock()
	var out []DiagnosticInfo
	for _, name := range rp.order {
		for _, d := range rp.prog.Diagnostics(rp.files[name]) {
			out = append(out, diagInfo(d))
		}
	}
	return out
}

func diagInfo(d *shim.Diagnostic) DiagnosticInfo {
	info := DiagnosticInfo{
		Code:     int(d.Code()),
		Category: mapCategory(d.Category()),
		Message:  shim.Message(d),
		Start:    d.Loc().Pos(),
		End:      d.Loc().End(),
	}
	if f := d.File(); f != nil {
		info.File = f.FileName()
	}
	for _, r := range d.MessageChain() {
		info.Related = append(info.Related, diagInfo(r))
	}
	return info
}

func (a *RealAdapter) LineColumnOf(p ProgramHandle, file string, at int) (line, column int) {
	sf := prog(p).files[file]
	if sf == nil {
		return 0, 0
	}
	l, c := shim.LineAndCharacter(sf, at)
	// Report a one-based line the way an editor does, keeping the column as the
	// zero-based UTF-16 offset the checker uses.
	return l + 1, c
}

func (a *RealAdapter) SourceFiles(p ProgramHandle) []NodeHandle {
	rp := prog(p)
	out := make([]NodeHandle, 0, len(rp.order))
	for _, name := range rp.order {
		out = append(out, rNode{n: rp.files[name].AsNode()})
	}
	return out
}

func (a *RealAdapter) KindOf(n NodeHandle) NodeKind { return mapKind(nodeOf(n).Kind) }

func (a *RealAdapter) ChildrenOf(n NodeHandle) []NodeHandle {
	var out []NodeHandle
	shim.ForEachChild(nodeOf(n), func(child *shim.Node) bool {
		out = append(out, rNode{n: child})
		return false
	})
	return out
}

func (a *RealAdapter) ForClausesOf(n NodeHandle) (init, cond, incr, body NodeHandle) {
	i, c, r, b := shim.ForClauses(nodeOf(n))
	wrap := func(x *shim.Node) NodeHandle {
		if x == nil {
			return nil
		}
		return rNode{n: x}
	}
	return wrap(i), wrap(c), wrap(r), wrap(b)
}

func (a *RealAdapter) SpanOf(n NodeHandle) (start, end int, file string) {
	node := nodeOf(n)
	return node.Pos(), node.End(), shim.FileName(node)
}

func (a *RealAdapter) TextOf(n NodeHandle) string { return shim.NodeText(nodeOf(n)) }

func (a *RealAdapter) FileKindOf(file string) FileKind {
	switch {
	case strings.HasSuffix(file, ".d.ts"):
		return FileDTS
	case strings.HasSuffix(file, ".tsx"):
		return FileTSX
	case strings.HasSuffix(file, ".ts"):
		return FileTS
	case strings.HasSuffix(file, ".jsx"):
		return FileJSX
	case strings.HasSuffix(file, ".js"), strings.HasSuffix(file, ".mjs"), strings.HasSuffix(file, ".cjs"):
		return FileJS
	case strings.HasSuffix(file, ".json"):
		return FileJSON
	default:
		return FileTS
	}
}

func (a *RealAdapter) Revision() string { return PinnedRevision }
