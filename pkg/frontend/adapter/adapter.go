// Package adapter is the only bento package that is allowed to import
// microsoft/typescript-go. TSAdapter is bento's contract for a TypeScript
// checker, and the real implementation over the pinned typescript-go revision
// lives behind it. Keeping this the sole coupling point is the whole mitigation
// for the pre-7.1 API churn described in 04_frontend_typescript_go.md section 3:
// when upstream renames a function or reshapes a type, exactly one package in
// bento changes.
//
// As of mid-2026 typescript-go exposes no public programmatic API at all: its
// checker, binder, and parser packages are under internal/, so no other module
// can import them. The real adapter is therefore blocked upstream until the
// stable API lands in TypeScript 7.1 (or bento ships a fork with a public shim).
// Until then the interface below is the stable seam every downstream pass codes
// against, and FakeAdapter (fake.go) is a working second implementation used by
// the partitioner and lowering tests, exactly the substitutability this design
// exists to provide.
package adapter

// ProgramHandle is an opaque handle to a built program. No typescript-go type
// appears on this interface; the unexported marker method means only this
// package can produce a value that satisfies it.
type ProgramHandle interface {
	programHandle()
}

// NodeHandle, TypeHandle, SymbolHandle, and SignatureHandle are opaque handles
// to the corresponding checker objects. They cross the adapter boundary as
// interfaces so the concrete typescript-go shapes never leak past it.
type NodeHandle interface{ nodeHandle() }
type TypeHandle interface{ typeHandle() }
type SymbolHandle interface{ symbolHandle() }
type SignatureHandle interface{ signatureHandle() }

// LiteralValue carries the concrete value of a literal type so lowering can fold
// closed unions into integer tags and refine integers. Only the field named by
// Kind is meaningful; BigInt holds the decimal digits so an arbitrary-precision
// literal survives the crossing without loss.
type LiteralValue struct {
	Kind   LiteralKind
	Str    string
	Num    float64
	Bool   bool
	BigInt string
}

// PropertyInfo is one member of an object type, as the adapter reports it. The
// type is an opaque handle the frontend wraps into a frontend.Type.
type PropertyInfo struct {
	Name     string
	Type     TypeHandle
	Optional bool
	Readonly bool
}

// ParamInfo is one parameter of a signature.
type ParamInfo struct {
	Name     string
	Type     TypeHandle
	Optional bool
}

// SignatureInfo is a call or construct signature in adapter terms.
type SignatureInfo struct {
	Params     []ParamInfo
	Return     TypeHandle
	TypeParams []TypeHandle
	RestParam  *ParamInfo
	MinArgs    int
}

// ResolvedImportInfo is one edge of the module graph as the checker resolved it.
type ResolvedImportInfo struct {
	Specifier    string
	ResolvedFile string
	Kind         ImportKind
}

// DiagnosticInfo is a checker or parser diagnostic, carried through unchanged so
// bento can render it byte for byte like tsc.
type DiagnosticInfo struct {
	Code     int
	Category DiagnosticCategory
	Message  string
	File     string
	Start    int
	End      int
	Related  []DiagnosticInfo
}

// CompilerOptions is the subset of tsconfig settings bento maps onto the
// checker. It is the resolved effective option set, after extends and overrides,
// that pkg/frontend hands the adapter to build a program. Emit options are
// absent because bento does not emit JavaScript.
type CompilerOptions struct {
	Strict           bool
	StrictNullChecks bool
	NoImplicitAny    bool
	Target           string
	Lib              []string
	Module           string
	ModuleResolution string
	AllowJS          bool
	CheckJS          bool
	JSX              string
	SkipLibCheck     bool
	BaseURL          string
	Paths            map[string][]string
}

// Host is how typescript-go reaches the file system and bento's module
// resolver. Routing both the checker and the runtime loader through one Host is
// what keeps the type view and the run view describing the same module graph
// (04_frontend_typescript_go.md section 5).
type Host interface {
	ReadFile(path string) (string, bool)
	FileExists(path string) bool
	DirectoryExists(path string) bool
	GetCurrentDirectory() string
	// ResolveModule maps a written specifier from a containing file to a
	// resolved absolute path and the kind of import it is. ok is false when the
	// specifier does not resolve.
	ResolveModule(specifier, containingFile string) (resolved string, kind ImportKind, ok bool)
}

// FileChange is one edit the watcher observed, fed to UpdateProgram for an
// incremental rebuild. Text, when set, is the new content so the editor path can
// hand a buffer in without a disk read.
type FileChange struct {
	Path string
	Kind ChangeKind
	Text string
}

// TSAdapter is the full set of operations bento performs against a TypeScript
// checker. It is exactly the adapter-method column of the section 11 table in
// 04_frontend_typescript_go.md. Every method takes and returns bento or opaque
// types, never a typescript-go type, so a pin move touches only the concrete
// implementation, never this contract.
type TSAdapter interface {
	// Construction.
	BuildProgram(roots []string, opts CompilerOptions, host Host) (ProgramHandle, error)
	UpdateProgram(prev ProgramHandle, changed []FileChange) (ProgramHandle, error)

	// Types by position and by symbol.
	TypeOfNode(p ProgramHandle, n NodeHandle) TypeHandle
	TypeOfSymbol(p ProgramHandle, s SymbolHandle) TypeHandle
	WidenType(p ProgramHandle, t TypeHandle) TypeHandle
	// DeclaredTypeOfNode returns the un-narrowed declared type of the variable
	// used at n, so the partitioner can tell when a branch relies on narrowing.
	DeclaredTypeOfNode(p ProgramHandle, n NodeHandle) (TypeHandle, bool)

	// Symbols.
	SymbolOfNode(p ProgramHandle, n NodeHandle) (SymbolHandle, bool)
	AliasedSymbol(p ProgramHandle, s SymbolHandle) SymbolHandle
	DeclarationsOf(p ProgramHandle, s SymbolHandle) []NodeHandle
	SymbolName(s SymbolHandle) string
	SymbolFlagsOf(s SymbolHandle) SymbolFlags

	// Signatures.
	ResolvedSignature(p ProgramHandle, call NodeHandle) (SignatureInfo, bool)
	SignaturesOf(p ProgramHandle, t TypeHandle) (call, construct []SignatureInfo)

	// Structural type queries.
	TypeFlagsOf(p ProgramHandle, t TypeHandle) TypeFlags
	UnionOf(p ProgramHandle, t TypeHandle) []TypeHandle
	PropertiesOf(p ProgramHandle, t TypeHandle) []PropertyInfo
	ElementOf(p ProgramHandle, t TypeHandle) (TypeHandle, bool)
	LiteralOf(p ProgramHandle, t TypeHandle) (LiteralValue, bool)

	// Module graph.
	ImportsOf(p ProgramHandle, file string) []ResolvedImportInfo

	// Diagnostics and positions.
	Diagnostics(p ProgramHandle) []DiagnosticInfo
	LineColumnOf(p ProgramHandle, file string, at int) (line, column int)

	// AST navigation, enough for lowering to walk a body.
	// SourceFiles returns the top-level node of each file in the program, the
	// roots the partitioner walks to enumerate units.
	SourceFiles(p ProgramHandle) []NodeHandle
	KindOf(n NodeHandle) NodeKind
	ChildrenOf(n NodeHandle) []NodeHandle
	SpanOf(n NodeHandle) (start, end int, file string)
	FileKindOf(file string) FileKind

	// Revision is the pinned typescript-go commit this adapter was built
	// against, asserted at init so a stray dependency bump cannot slip past.
	Revision() string
}
