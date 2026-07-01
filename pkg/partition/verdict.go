package partition

import "github.com/tamnd/bento/pkg/frontend"

// Verdict is what the partitioner assigns to a unit. The three values are the
// whole vocabulary of 06_compile_vs_interpret.md section 3.
type Verdict int

const (
	// Compiled means the unit is lowered to Go and runs native, with no engine,
	// no boxing, and no guard.
	Compiled Verdict = iota
	// Interpreted means the unit runs on the engine.
	Interpreted
	// Speculative means the unit is lowered under one or more type assumptions,
	// each protected by a guard, with a defined path back to the engine when a
	// guard fails. Pass A never assigns this; it is a Pass C promotion of a
	// soft-blocked unit.
	Speculative
)

func (v Verdict) String() string {
	switch v {
	case Compiled:
		return "Compiled"
	case Interpreted:
		return "Interpreted"
	case Speculative:
		return "Speculative"
	default:
		return "Unknown"
	}
}

// Severity separates the two kinds of blocker. A hard blocker can never be
// compiled and no guard rescues it, so a unit that hits one is Interpreted and
// stays there. A soft blocker blocks plain compilation but leaves the unit a
// Pass C speculation candidate.
type Severity int

const (
	// Soft blockers reach Pass C as speculation candidates.
	Soft Severity = iota
	// Hard blockers set Interpreted in Pass A and no later pass moves it.
	Hard
)

func (s Severity) String() string {
	if s == Hard {
		return "Hard"
	}
	return "Soft"
}

// ReasonKind names why a unit failed local classification. The set is closed
// here and grows only as the spec's blocker lists (sections 6.1 and 6.2) are
// covered. Each kind has a fixed severity, returned by Severity.
type ReasonKind int

const (
	// ReasonEval: the unit calls eval, which defeats static scoping. Hard.
	ReasonEval ReasonKind = iota
	// ReasonWith: the unit uses a with statement, which defeats static scoping.
	// Hard.
	ReasonWith
	// ReasonNewFunction: the unit builds a function from a string with
	// new Function(...), whose body cannot be checked or lowered. Hard.
	ReasonNewFunction
	// ReasonProtoMutation: the unit reflectively changes a value's type identity
	// (__proto__ assignment, Object.setPrototypeOf). Hard.
	ReasonProtoMutation
	// ReasonProxyTrap: the unit reads or writes through a Proxy trap layer, where
	// every operation could run arbitrary user code. Hard.
	ReasonProxyTrap
	// ReasonArgumentsAliasing: the unit manipulates the arguments object in a way
	// that aliases parameters. Hard.
	ReasonArgumentsAliasing
	// ReasonPropertyMutation: the unit adds a property a fixed object or class
	// shape never declared, or deletes one it did. Hard for that unit; the shape
	// no longer has a sound fixed Go layout.
	ReasonPropertyMutation
	// ReasonUntypedValue: an any or unknown value is observable at a point where
	// the unit operates on it without first narrowing to a lowerable type. Soft:
	// monomorphic call sites can justify a guarded speculation.
	ReasonUntypedValue
	// ReasonUnlowerableType: a binding, parameter, or return has a type outside
	// the lowerable set of 05_type_lowering.md (a bare type parameter, an
	// intersection, a shape lowering does not render yet). Soft.
	ReasonUnlowerableType
	// ReasonUnsupportedSyntax: the unit uses a language construct the lowering
	// pass does not cover yet. Soft, because the covered set grows each release.
	ReasonUnsupportedSyntax
)

// Severity returns the fixed severity of a reason kind. This is the single table
// that decides whether a reason is a hard block or a speculation candidate.
func (k ReasonKind) Severity() Severity {
	switch k {
	case ReasonEval, ReasonWith, ReasonNewFunction, ReasonProtoMutation,
		ReasonProxyTrap, ReasonArgumentsAliasing, ReasonPropertyMutation:
		return Hard
	default:
		return Soft
	}
}

func (k ReasonKind) String() string {
	switch k {
	case ReasonEval:
		return "eval"
	case ReasonWith:
		return "with"
	case ReasonNewFunction:
		return "new Function"
	case ReasonProtoMutation:
		return "prototype mutation"
	case ReasonProxyTrap:
		return "proxy trap"
	case ReasonArgumentsAliasing:
		return "arguments aliasing"
	case ReasonPropertyMutation:
		return "property add or delete on a fixed shape"
	case ReasonUntypedValue:
		return "untyped value used without narrowing"
	case ReasonUnlowerableType:
		return "type outside the lowerable set"
	case ReasonUnsupportedSyntax:
		return "unsupported syntax"
	default:
		return "unknown"
	}
}

// Reason is one recorded cause for a unit not being cleanly Compiled. It carries
// the node it was found at so a diagnostic can point precisely, and a short
// human message. Pass C reads Kind and Severity to decide whether the unit is a
// speculation candidate.
type Reason struct {
	Kind    ReasonKind
	Node    frontend.Node
	Message string
}

// Severity is the severity of the reason's kind.
func (r Reason) Severity() Severity { return r.Kind.Severity() }
