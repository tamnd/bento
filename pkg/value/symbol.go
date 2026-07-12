// This file owns Symbol, the reference behind a KindSymbol value. A symbol is a
// unique, immutable primitive whose only observable state is an optional
// description string; two symbols are equal only when they are the same
// reference, never by description. That identity is what lets a symbol serve as a
// property key that cannot collide with a string key or with another symbol, which
// is why the dynamic object keys its bag by string and symbol alike.

package value

import "unsafe"

// Symbol is the storage behind a KindSymbol value. It carries only the
// description a Symbol(desc) call recorded, used by toString and the
// description getter; identity lives in the pointer itself, so the box's ref
// distinguishes one symbol from another and StrictEquals compares symbols by that
// pointer the way the language compares them by reference. The hasDesc flag keeps
// Symbol() apart from Symbol(""): the former's description is undefined while the
// latter's is the empty string, a difference the empty BStr alone cannot record.
type Symbol struct {
	desc    BStr
	hasDesc bool
}

// NewSymbol boxes a fresh symbol with the given description, the value a
// Symbol(desc) call produces. Each call allocates a new Symbol, so two calls with
// the same description are still distinct references and never compare equal, the
// uniqueness the language guarantees.
func NewSymbol(desc BStr) Value {
	return Value{kind: KindSymbol, ref: unsafe.Pointer(&Symbol{desc: desc, hasDesc: true})}
}

// NewSymbolNoDesc boxes a fresh symbol created without a description, the value a
// bare Symbol() call produces, whose description reads back as undefined.
func NewSymbolNoDesc() Value {
	return Value{kind: KindSymbol, ref: unsafe.Pointer(&Symbol{})}
}

// symbol returns the *Symbol a symbol box holds, the reference used as an
// identity key in the property bag and compared by pointer in StrictEquals.
func (v Value) symbol() *Symbol { return (*Symbol)(v.ref) }

// symbolValue boxes an existing *Symbol back into a value, the reverse of symbol,
// so a walk over an object's symbol-keyed properties can hand each key to an API
// that takes a boxed key such as DefineProperty.
func symbolValue(s *Symbol) Value {
	return Value{kind: KindSymbol, ref: unsafe.Pointer(s)}
}

// SymbolDescription returns the symbol's description as a string value, or
// undefined when it was created without one, the read Symbol.prototype.description
// makes. It is only valid on a KindSymbol value.
func (v Value) SymbolDescription() Value {
	s := v.symbol()
	if !s.hasDesc {
		return Undefined
	}
	return StringValue(s.desc)
}

// symbolRegistry backs the global symbol registry Symbol.for and Symbol.keyFor
// share. It maps a string key to the one symbol that key names, so every
// Symbol.for("k") returns the same reference, the cross-realm identity the
// registry guarantees. symbolRegistryKeys is the reverse map Symbol.keyFor
// reads, recording the key each registered symbol was interned under so a symbol
// can report the string that owns it. A program compiled by bento runs one
// agent with no shared-memory concurrency, so a plain map needs no lock.
var symbolRegistry = map[string]*Symbol{}
var symbolRegistryKeys = map[*Symbol]BStr{}

// SymbolFor returns the registered symbol for key, creating and interning one
// when the key is new, the value Symbol.for(key) produces. A registered symbol's
// description is its key, matching the specification, and every call with an
// equal key returns the same reference so Symbol.for("k") === Symbol.for("k").
func SymbolFor(key BStr) Value {
	k := key.ToGoString()
	if s, ok := symbolRegistry[k]; ok {
		return symbolValue(s)
	}
	s := &Symbol{desc: key, hasDesc: true}
	symbolRegistry[k] = s
	symbolRegistryKeys[s] = key
	return symbolValue(s)
}

// SymbolKeyFor returns the registry key a symbol was interned under as a string
// value, or undefined when the symbol never entered the registry, the read
// Symbol.keyFor(sym) makes. It is only valid on a KindSymbol value, the shape the
// lowerer guarantees at the call site.
func SymbolKeyFor(v Value) Value {
	if key, ok := symbolRegistryKeys[v.symbol()]; ok {
		return StringValue(key)
	}
	return Undefined
}

// SymbolDescriptiveString renders a symbol as "Symbol(desc)", the SymbolDescriptiveString
// abstract operation Symbol.prototype.toString returns. A symbol with no description
// reads as "Symbol()", since a missing description contributes the empty string
// between the parentheses. It is only valid on a KindSymbol value.
func (v Value) SymbolDescriptiveString() BStr {
	s := v.symbol()
	desc := s.desc
	if !s.hasDesc {
		desc = FromGoString("")
	}
	return FromGoString("Symbol(").ConcatN(desc, FromGoString(")"))
}
