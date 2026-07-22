package frontend

import "github.com/tamnd/bento/pkg/frontend/adapter"

// This file defines bento's own vocabulary for a typed program: the value types
// the partitioner (06_compile_vs_interpret.md) and lowering
// (05_type_lowering.md) consume. Every one of them is a bento type, never a
// typescript-go type, so downstream code is insulated from the upstream API
// churn described in 04_frontend_typescript_go.md section 3. The classification
// enums are owned by the adapter package and aliased here so a consumer names
// frontend.TypeNumber, and a pin move that reshuffles the checker never reaches
// past pkg/frontend/adapter.

// Classification enum aliases. The underlying values live in the adapter package.
type (
	TypeFlags          = adapter.TypeFlags
	SymbolFlags        = adapter.SymbolFlags
	NodeKind           = adapter.NodeKind
	FileKind           = adapter.FileKind
	ImportKind         = adapter.ImportKind
	DiagnosticCategory = adapter.DiagnosticCategory
	LiteralKind        = adapter.LiteralKind
	LiteralValue       = adapter.LiteralValue
)

// TypeFlags values.
const (
	TypeAny           = adapter.TypeAny
	TypeUnknown       = adapter.TypeUnknown
	TypeNever         = adapter.TypeNever
	TypeVoid          = adapter.TypeVoid
	TypeUndefined     = adapter.TypeUndefined
	TypeNull          = adapter.TypeNull
	TypeBoolean       = adapter.TypeBoolean
	TypeNumber        = adapter.TypeNumber
	TypeBigInt        = adapter.TypeBigInt
	TypeString        = adapter.TypeString
	TypeSymbol        = adapter.TypeSymbol
	TypeLiteral       = adapter.TypeLiteral
	TypeObject        = adapter.TypeObject
	TypeUnion         = adapter.TypeUnion
	TypeIntersection  = adapter.TypeIntersection
	TypeTypeParameter = adapter.TypeTypeParameter
	TypeEnum          = adapter.TypeEnum
)

// NodeKind values. The full set lives in the adapter package; these are the
// kinds the partitioner and lowering branch on today, aliased so a consumer
// names frontend.NodeFunctionDeclaration.
const (
	NodeUnknown = adapter.NodeUnknown

	NodeSourceFile           = adapter.NodeSourceFile
	NodeFunctionDeclaration  = adapter.NodeFunctionDeclaration
	NodeFunctionExpression   = adapter.NodeFunctionExpression
	NodeArrowFunction        = adapter.NodeArrowFunction
	NodeMethodDeclaration    = adapter.NodeMethodDeclaration
	NodeGetAccessor          = adapter.NodeGetAccessor
	NodeSetAccessor          = adapter.NodeSetAccessor
	NodeConstructor          = adapter.NodeConstructor
	NodeClassDeclaration     = adapter.NodeClassDeclaration
	NodeInterfaceDeclaration = adapter.NodeInterfaceDeclaration
	NodeTypeAliasDeclaration = adapter.NodeTypeAliasDeclaration
	NodeEnumDeclaration      = adapter.NodeEnumDeclaration
	NodeVariableStatement    = adapter.NodeVariableStatement
	NodeVariableDeclaration  = adapter.NodeVariableDeclaration
	NodeParameter            = adapter.NodeParameter
	NodePropertyDeclaration  = adapter.NodePropertyDeclaration

	NodeBlock               = adapter.NodeBlock
	NodeReturnStatement     = adapter.NodeReturnStatement
	NodeIfStatement         = adapter.NodeIfStatement
	NodeForStatement        = adapter.NodeForStatement
	NodeForOfStatement      = adapter.NodeForOfStatement
	NodeForInStatement      = adapter.NodeForInStatement
	NodeWhileStatement      = adapter.NodeWhileStatement
	NodeSwitchStatement     = adapter.NodeSwitchStatement
	NodeTryStatement        = adapter.NodeTryStatement
	NodeThrowStatement      = adapter.NodeThrowStatement
	NodeExpressionStatement = adapter.NodeExpressionStatement

	NodeIdentifier               = adapter.NodeIdentifier
	NodeCallExpression           = adapter.NodeCallExpression
	NodeNewExpression            = adapter.NodeNewExpression
	NodePropertyAccessExpression = adapter.NodePropertyAccessExpression
	NodeElementAccessExpression  = adapter.NodeElementAccessExpression
	NodeBinaryExpression         = adapter.NodeBinaryExpression
	NodePrefixUnaryExpression    = adapter.NodePrefixUnaryExpression
	NodePostfixUnaryExpression   = adapter.NodePostfixUnaryExpression
	NodeConditionalExpression    = adapter.NodeConditionalExpression
	NodeTemplateExpression       = adapter.NodeTemplateExpression
	NodeObjectLiteralExpression  = adapter.NodeObjectLiteralExpression
	NodeArrayLiteralExpression   = adapter.NodeArrayLiteralExpression
	NodeAwaitExpression          = adapter.NodeAwaitExpression
	NodeYieldExpression          = adapter.NodeYieldExpression
	NodeSpreadElement            = adapter.NodeSpreadElement
	NodeParenthesizedExpression  = adapter.NodeParenthesizedExpression
	NodeAsExpression             = adapter.NodeAsExpression
	NodeTypeAssertion            = adapter.NodeTypeAssertion

	// Literals and keyword-valued expressions.
	NodeNumericLiteral                = adapter.NodeNumericLiteral
	NodeStringLiteral                 = adapter.NodeStringLiteral
	NodeBigIntLiteral                 = adapter.NodeBigIntLiteral
	NodeNoSubstitutionTemplateLiteral = adapter.NodeNoSubstitutionTemplateLiteral
	NodeTrueKeyword                   = adapter.NodeTrueKeyword
	NodeFalseKeyword                  = adapter.NodeFalseKeyword
	NodeNullKeyword                   = adapter.NodeNullKeyword

	// Class-body keywords.
	NodeThisKeyword  = adapter.NodeThisKeyword
	NodeSuperKeyword = adapter.NodeSuperKeyword

	NodeWithStatement = adapter.NodeWithStatement
)

// SymbolFlags values.
const (
	SymbolVariable  = adapter.SymbolVariable
	SymbolFunction  = adapter.SymbolFunction
	SymbolClass     = adapter.SymbolClass
	SymbolInterface = adapter.SymbolInterface
	SymbolTypeAlias = adapter.SymbolTypeAlias
	SymbolEnum      = adapter.SymbolEnum
	SymbolNamespace = adapter.SymbolNamespace
	SymbolMethod    = adapter.SymbolMethod
	SymbolProperty  = adapter.SymbolProperty
	SymbolAlias     = adapter.SymbolAlias
)

// FileKind values.
const (
	FileTS   = adapter.FileTS
	FileTSX  = adapter.FileTSX
	FileJS   = adapter.FileJS
	FileJSX  = adapter.FileJSX
	FileDTS  = adapter.FileDTS
	FileJSON = adapter.FileJSON
)

// ImportKind values.
const (
	ImportRelative = adapter.ImportRelative
	ImportBare     = adapter.ImportBare
	ImportNode     = adapter.ImportNode
	ImportGo       = adapter.ImportGo
	ImportJSON     = adapter.ImportJSON
	ImportAsset    = adapter.ImportAsset
)

// DiagnosticCategory values.
const (
	CategoryError      = adapter.CategoryError
	CategoryWarning    = adapter.CategoryWarning
	CategorySuggestion = adapter.CategorySuggestion
	CategoryMessage    = adapter.CategoryMessage
)

// LiteralKind values.
const (
	LiteralNone    = adapter.LiteralNone
	LiteralString  = adapter.LiteralString
	LiteralNumber  = adapter.LiteralNumber
	LiteralBoolean = adapter.LiteralBoolean
	LiteralBigInt  = adapter.LiteralBigInt
)

// Pos is a byte offset into a file's text.
type Pos int

// Position is a resolved line and column: 1-based line, 0-based column, the
// convention tsc uses so bento diagnostics line up with it.
type Position struct {
	Line   int
	Column int
}

// Span is a half-open byte range in a file.
type Span struct {
	Start Pos
	End   Pos
}

// SourceFile identifies a file in the program and lets consumers turn offsets
// into human positions for diagnostics and source maps.
type SourceFile struct {
	Path string
	Kind FileKind
}

// Node is an opaque handle to an AST node. Consumers ask the Program about it
// rather than reading its fields, so the concrete typescript-go node shape stays
// behind the adapter.
type Node interface {
	Kind() NodeKind
	Pos() Pos
	End() Pos
	File() SourceFile
}

// ForClauses is a for statement's four parts read by role. Body is always
// present. Init, Cond, and Incr each carry a node only when the source wrote
// that clause, reported by the matching HasInit, HasCond, HasIncr flag, so a
// caller lowers for(;;), for(let i=0;;i++), and every other shape without
// guessing which clauses a bare child list dropped.
type ForClauses struct {
	Init, Cond, Incr, Body    Node
	HasInit, HasCond, HasIncr bool
}

// Type is bento's view of a TypeScript type: a small handle carrying the coarse
// flags eagerly, with structural detail fetched through Program methods so the
// typescript-go type object never crosses the adapter boundary.
type Type struct {
	Flags TypeFlags
	id    typeID
}

// Identity returns a small integer stable within one Program that is equal for
// two Type values naming the same underlying type. It is enough to break cycles
// when a consumer walks a recursive type (an object whose field type refers back
// to itself) without exposing the typescript-go type object. Two Type values
// from different programs may share an id, so use it only within one program's
// traversal.
func (t Type) Identity() int { return int(t.id) }

// Symbol is bento's view of a bound declaration. Identity is by the opaque id,
// so two Symbol values name the same symbol when their ids are equal.
type Symbol struct {
	Name  string
	Flags SymbolFlags
	id    symbolID
}

// Property is one member of an object type.
type Property struct {
	Name     string
	Type     Type
	Optional bool
	Readonly bool
	// WriteType is the type a write to the member accepts. It equals Type for a
	// plain property and widens to the setter type for a divergent accessor.
	WriteType Type
	// DivergentAccessor is true when the member is a get/set accessor pair whose
	// write type differs from its read type.
	DivergentAccessor bool
}

// Param is one parameter of a signature.
type Param struct {
	Name     string
	Type     Type
	Optional bool
}

// TupleElem is one positional element of a tuple type: its element type, whether
// the position is optional or the rest tail, and its label when the source named
// it. Lowering reads the slice to spell the positional struct a tuple maps to.
type TupleElem struct {
	Type     Type
	Optional bool
	Rest     bool
	Label    string
}

// Signature is bento's view of a call or construct signature. Lowering reads it
// to emit a Go function signature; the partitioner reads it to decide whether
// every parameter and the return are lowerable.
type Signature struct {
	Params     []Param
	Return     Type
	TypeParams []Type
	RestParam  *Param
	MinArgs    int
}

// ResolvedImport is one edge in the module graph.
type ResolvedImport struct {
	Specifier string
	Resolved  SourceFile
	Kind      ImportKind
}

// Diagnostic is a checker or parser diagnostic, passed through from
// typescript-go without rewording so bento errors match tsc byte for byte.
type Diagnostic struct {
	Code     int
	Category DiagnosticCategory
	Message  string
	File     *SourceFile
	Span     Span
	Related  []Diagnostic
}
