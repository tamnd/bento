package adapter

// This file maps the checker's own classification enums onto bento's coarse
// enums. The checker distinguishes far more cases than bento branches on, so
// each mapping collapses a family of checker flags to the one bento facet that
// drives partitioning and lowering. The tables live beside RealAdapter because
// they cross the typescript-go seam and so must stay inside this package.

import "github.com/microsoft/typescript-go/shim"

// mapTypeFlags collapses the checker's type flags to bento's. A literal keeps
// both its base primitive facet and the TypeLiteral bit, because lowering treats
// the literal 42 as a number that also carries a concrete value.
func mapTypeFlags(f shim.TypeFlags) TypeFlags {
	var out TypeFlags
	set := func(mask shim.TypeFlags, bit TypeFlags) {
		if f&mask != 0 {
			out |= bit
		}
	}
	set(shim.TypeFlagsAny, TypeAny)
	set(shim.TypeFlagsUnknown, TypeUnknown)
	set(shim.TypeFlagsNever, TypeNever)
	set(shim.TypeFlagsVoid, TypeVoid)
	set(shim.TypeFlagsUndefined, TypeUndefined)
	set(shim.TypeFlagsNull, TypeNull)
	set(shim.TypeFlagsBoolean|shim.TypeFlagsBooleanLiteral, TypeBoolean)
	set(shim.TypeFlagsNumber|shim.TypeFlagsNumberLiteral, TypeNumber)
	set(shim.TypeFlagsBigInt|shim.TypeFlagsBigIntLiteral, TypeBigInt)
	set(shim.TypeFlagsString|shim.TypeFlagsStringLiteral, TypeString)
	set(shim.TypeFlagsESSymbol, TypeSymbol)
	set(shim.TypeFlagsLiteral, TypeLiteral)
	set(shim.TypeFlagsObject, TypeObject)
	set(shim.TypeFlagsUnion, TypeUnion)
	set(shim.TypeFlagsIntersection, TypeIntersection)
	set(shim.TypeFlagsTypeParameter, TypeTypeParameter)
	set(shim.TypeFlagsEnumLike, TypeEnum)
	return out
}

// mapSymbolFlags collapses the checker's symbol flags to bento's.
func mapSymbolFlags(f shim.SymbolFlags) SymbolFlags {
	var out SymbolFlags
	set := func(mask shim.SymbolFlags, bit SymbolFlags) {
		if f&mask != 0 {
			out |= bit
		}
	}
	set(shim.SymbolFlagsVariable, SymbolVariable)
	set(shim.SymbolFlagsFunction, SymbolFunction)
	set(shim.SymbolFlagsClass, SymbolClass)
	set(shim.SymbolFlagsInterface, SymbolInterface)
	set(shim.SymbolFlagsTypeAlias, SymbolTypeAlias)
	set(shim.SymbolFlagsEnum, SymbolEnum)
	set(shim.SymbolFlagsMethod, SymbolMethod)
	set(shim.SymbolFlagsProperty, SymbolProperty)
	set(shim.SymbolFlagsAlias, SymbolAlias)
	set(shim.SymbolFlagsNamespace, SymbolNamespace)
	return out
}

// mapCategory maps a diagnostic severity across the seam.
func mapCategory(c shim.DiagnosticCategory) DiagnosticCategory {
	switch c {
	case shim.DiagnosticCategoryWarning:
		return CategoryWarning
	case shim.DiagnosticCategorySuggestion:
		return CategorySuggestion
	case shim.DiagnosticCategoryMessage:
		return CategoryMessage
	default:
		return CategoryError
	}
}

// mapKind maps a checker syntax kind to bento's node kind. A kind bento does not
// name yet maps to NodeUnknown, which a walker treats as an opaque node to
// descend into rather than a construct to lower.
func mapKind(k shim.Kind) NodeKind {
	switch k {
	case shim.KindSourceFile:
		return NodeSourceFile
	case shim.KindFunctionDeclaration:
		return NodeFunctionDeclaration
	case shim.KindFunctionExpression:
		return NodeFunctionExpression
	case shim.KindArrowFunction:
		return NodeArrowFunction
	case shim.KindMethodDeclaration:
		return NodeMethodDeclaration
	case shim.KindGetAccessor:
		return NodeGetAccessor
	case shim.KindSetAccessor:
		return NodeSetAccessor
	case shim.KindConstructor:
		return NodeConstructor
	case shim.KindClassDeclaration:
		return NodeClassDeclaration
	case shim.KindInterfaceDeclaration:
		return NodeInterfaceDeclaration
	case shim.KindTypeAliasDeclaration:
		return NodeTypeAliasDeclaration
	case shim.KindEnumDeclaration:
		return NodeEnumDeclaration
	case shim.KindVariableStatement:
		return NodeVariableStatement
	case shim.KindVariableDeclaration:
		return NodeVariableDeclaration
	case shim.KindParameter:
		return NodeParameter
	case shim.KindPropertyDeclaration:
		return NodePropertyDeclaration
	case shim.KindBlock:
		return NodeBlock
	case shim.KindReturnStatement:
		return NodeReturnStatement
	case shim.KindIfStatement:
		return NodeIfStatement
	case shim.KindForStatement:
		return NodeForStatement
	case shim.KindForOfStatement:
		return NodeForOfStatement
	case shim.KindForInStatement:
		return NodeForInStatement
	case shim.KindWhileStatement:
		return NodeWhileStatement
	case shim.KindSwitchStatement:
		return NodeSwitchStatement
	case shim.KindTryStatement:
		return NodeTryStatement
	case shim.KindThrowStatement:
		return NodeThrowStatement
	case shim.KindExpressionStatement:
		return NodeExpressionStatement
	case shim.KindWithStatement:
		return NodeWithStatement
	case shim.KindIdentifier:
		return NodeIdentifier
	case shim.KindCallExpression:
		return NodeCallExpression
	case shim.KindNewExpression:
		return NodeNewExpression
	case shim.KindPropertyAccessExpression:
		return NodePropertyAccessExpression
	case shim.KindElementAccessExpression:
		return NodeElementAccessExpression
	case shim.KindBinaryExpression:
		return NodeBinaryExpression
	case shim.KindPrefixUnaryExpression:
		return NodePrefixUnaryExpression
	case shim.KindPostfixUnaryExpression:
		return NodePostfixUnaryExpression
	case shim.KindConditionalExpression:
		return NodeConditionalExpression
	case shim.KindTemplateExpression:
		return NodeTemplateExpression
	case shim.KindObjectLiteralExpression:
		return NodeObjectLiteralExpression
	case shim.KindArrayLiteralExpression:
		return NodeArrayLiteralExpression
	case shim.KindAwaitExpression:
		return NodeAwaitExpression
	case shim.KindYieldExpression:
		return NodeYieldExpression
	case shim.KindSpreadElement:
		return NodeSpreadElement
	case shim.KindParenthesizedExpression:
		return NodeParenthesizedExpression
	case shim.KindNumericLiteral:
		return NodeNumericLiteral
	case shim.KindStringLiteral:
		return NodeStringLiteral
	case shim.KindBigIntLiteral:
		return NodeBigIntLiteral
	case shim.KindNoSubstitutionTemplateLiteral:
		return NodeNoSubstitutionTemplateLiteral
	case shim.KindTrueKeyword:
		return NodeTrueKeyword
	case shim.KindFalseKeyword:
		return NodeFalseKeyword
	case shim.KindNullKeyword:
		return NodeNullKeyword
	case shim.KindThisKeyword:
		return NodeThisKeyword
	case shim.KindSuperKeyword:
		return NodeSuperKeyword
	default:
		return NodeUnknown
	}
}
