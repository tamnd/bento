package adapter

// This file holds the coarse classification enums bento uses to describe a
// typed program. They live in the adapter package because the TSAdapter
// interface returns them, and pkg/frontend aliases each one so a consumer names
// frontend.TypeNumber, not adapter.TypeNumber. Defining them once here keeps the
// adapter the sole owner of every value that crosses the typescript-go seam,
// per 04_frontend_typescript_go.md section 11.

// TypeFlags classify a type at the level the partitioner and lowering branch on.
// They are bit flags because a checker type can carry more than one facet (a
// literal is also its base primitive), so a consumer tests with a mask rather
// than an equality.
type TypeFlags uint32

const (
	// TypeAny is the untyped top; operating on it forces interpretation.
	TypeAny TypeFlags = 1 << iota
	// TypeUnknown is the safe top; lowerable only after a narrow.
	TypeUnknown
	// TypeNever is the uninhabited bottom; its slots carry no representation.
	TypeNever
	// TypeVoid is the absence of a return value.
	TypeVoid
	// TypeUndefined is the undefined sentinel type.
	TypeUndefined
	// TypeNull is the null sentinel type.
	TypeNull
	// TypeBoolean lowers to Go bool.
	TypeBoolean
	// TypeNumber lowers to float64, refined to int32/int64 when provably safe.
	TypeNumber
	// TypeBigInt lowers to *big.Int.
	TypeBigInt
	// TypeString lowers to the UTF-16 bstr.
	TypeString
	// TypeSymbol lowers to *value.Symbol, identity by pointer.
	TypeSymbol
	// TypeLiteral marks a literal type such as 42, "on", or true. It is always
	// combined with the base primitive flag it refines.
	TypeLiteral
	// TypeObject covers classes, interfaces, object literals, arrays, and
	// functions; the structural queries tell the shapes apart.
	TypeObject
	// TypeUnion marks a union whose constituents come from UnionOf.
	TypeUnion
	// TypeIntersection marks an intersection, lowered to a merged struct.
	TypeIntersection
	// TypeTypeParameter marks an un-instantiated generic parameter.
	TypeTypeParameter
	// TypeEnum marks an enum type, lowered per section 18.
	TypeEnum
)

// SymbolFlags classify what a bound name refers to. Like TypeFlags they are bit
// flags because TypeScript merges declarations, so one symbol can be several
// things at once (a class is both a value and a type).
type SymbolFlags uint32

const (
	// SymbolVariable is a let, const, or var binding.
	SymbolVariable SymbolFlags = 1 << iota
	// SymbolFunction is a function declaration or expression.
	SymbolFunction
	// SymbolClass is a class declaration.
	SymbolClass
	// SymbolInterface is an interface declaration.
	SymbolInterface
	// SymbolTypeAlias is a type alias.
	SymbolTypeAlias
	// SymbolEnum is an enum declaration.
	SymbolEnum
	// SymbolNamespace is a namespace or module.
	SymbolNamespace
	// SymbolMethod is a class or object method.
	SymbolMethod
	// SymbolProperty is a class field or object property.
	SymbolProperty
	// SymbolAlias is an import or export alias that points at another symbol.
	SymbolAlias
)

// NodeKind is bento's own enumeration of AST node kinds, mapped from
// typescript-go's syntax kinds by the adapter's convert table. Lowering walks a
// body by asking KindOf, so this set names every node kind the partitioner and
// lowering branch on. It grows as lowering coverage grows.
type NodeKind int

const (
	// NodeUnknown is the fallback for a kind bento does not name yet.
	NodeUnknown NodeKind = iota

	// Declarations.
	NodeSourceFile
	NodeFunctionDeclaration
	NodeFunctionExpression
	NodeArrowFunction
	NodeMethodDeclaration
	NodeGetAccessor
	NodeSetAccessor
	NodeConstructor
	NodeClassDeclaration
	NodeInterfaceDeclaration
	NodeTypeAliasDeclaration
	NodeEnumDeclaration
	NodeVariableStatement
	NodeVariableDeclaration
	NodeParameter
	NodePropertyDeclaration

	// Statements.
	NodeBlock
	NodeReturnStatement
	NodeIfStatement
	NodeForStatement
	NodeForOfStatement
	NodeForInStatement
	NodeWhileStatement
	NodeSwitchStatement
	NodeTryStatement
	NodeThrowStatement
	NodeExpressionStatement

	// Expressions.
	NodeIdentifier
	NodeCallExpression
	NodeNewExpression
	NodePropertyAccessExpression
	NodeElementAccessExpression
	NodeBinaryExpression
	NodePrefixUnaryExpression
	NodePostfixUnaryExpression
	NodeConditionalExpression
	NodeTemplateExpression
	NodeObjectLiteralExpression
	NodeArrayLiteralExpression
	NodeAwaitExpression
	NodeYieldExpression
	NodeSpreadElement
	NodeParenthesizedExpression

	// Literals and keyword-valued expressions, the leaves lowering reads a
	// constant from.
	NodeNumericLiteral
	NodeStringLiteral
	NodeBigIntLiteral
	NodeNoSubstitutionTemplateLiteral
	NodeTrueKeyword
	NodeFalseKeyword
	NodeNullKeyword

	// Class-body keywords: the receiver reference inside a method and the
	// parent reference inside a subclass.
	NodeThisKeyword
	NodeSuperKeyword

	// Constructs the partitioner treats as hard blockers when present.
	NodeWithStatement
)

// FileKind records what a source file is so the loader and partitioner route it.
type FileKind int

const (
	FileTS FileKind = iota
	FileTSX
	FileJS
	FileJSX
	FileDTS
	FileJSON
)

// ImportKind marks where a resolved import points so the loader and interop
// generator bind it correctly.
type ImportKind int

const (
	ImportRelative ImportKind = iota
	ImportBare
	ImportNode
	ImportGo
	ImportJSON
	ImportAsset
)

// DiagnosticCategory is the severity typescript-go assigns a diagnostic, passed
// through verbatim so bento output matches tsc.
type DiagnosticCategory int

const (
	CategoryError DiagnosticCategory = iota
	CategoryWarning
	CategorySuggestion
	CategoryMessage
)

// LiteralKind tags which primitive a LiteralValue carries.
type LiteralKind int

const (
	LiteralNone LiteralKind = iota
	LiteralString
	LiteralNumber
	LiteralBoolean
	LiteralBigInt
)

// ChangeKind is the sort of edit the watcher observed for an incremental update.
type ChangeKind int

const (
	ChangeCreated ChangeKind = iota
	ChangeModified
	ChangeDeleted
)
