// This file is the collection half of the deep equality port, node's setEquiv,
// mapEquiv and the three helpers they lean on (lib/internal/util/comparisons.js). It
// is separate from nodedeepequal.go only for size: the two sets of rules are one
// algorithm, and every comment here is about which of node's steps a line is.
//
// The shape of both walks is the same. Two collections of the same size are equal
// when their members pair off, and the hard half is that a member is not looked up by
// identity but by deep equality: a set holding one object matches a set holding a
// different object with the same properties. So the walk first settles every member
// that can be settled by a direct lookup, collects the rest, and then hunts for a deep
// match for each of those, removing the partner it consumed so two equal members on
// one side need two on the other.

package value

import "math"

// deepSetEquiv is node's setEquiv. The first pass over the first set files each
// object member for the pairwise hunt, since two deeply equal objects are not the same
// member and a direct has() would miss them, and settles each primitive member with a
// direct has(). The second pass runs only when something was filed, and the sets are
// equal when the filing is exhausted by the second set's members.
func deepSetEquiv(a, b setBacking, mode deepMode, memos *deepMemos) bool {
	strict := mode != deepLoose
	var pending []Value
	for i := 0; i < a.jsSize(); i++ {
		v := a.jsMember(i)
		if deepIsObject(v) {
			pending = append(pending, v)
			continue
		}
		if b.jsHas(v) {
			continue
		}
		if strict {
			return false
		}
		// A primitive the second set does not hold can still match loosely, 1 against
		// "1" or null against undefined, but only when such a partner could exist at
		// all. The probe rejects the members that cannot have one, which is what keeps
		// the loose comparison from filing every missing primitive for the hunt.
		if !deepSetMightHaveLoosePrim(a, b, v) {
			return false
		}
		pending = append(pending, v)
	}
	if pending == nil {
		return true
	}
	for i := 0; i < b.jsSize(); i++ {
		v := b.jsMember(i)
		if deepIsObject(v) {
			var ok bool
			if pending, ok = deepTakeEqual(pending, v, mode, memos); !ok {
				return false
			}
			continue
		}
		if strict || a.jsHas(v) {
			continue
		}
		var ok bool
		if pending, ok = deepTakeEqual(pending, v, mode, memos); !ok {
			return false
		}
	}
	return len(pending) == 0
}

// deepMapEquiv is node's mapEquiv, setEquiv with a value hanging off each key. An
// object key is filed for the hunt for the reason an object member is; a primitive key
// is settled by reading the second map's value for it, which is one lookup rather than
// the has-then-get pair.
func deepMapEquiv(a, b mapBacking, mode deepMode, memos *deepMemos) bool {
	strict := mode != deepLoose
	var pending []Value
	for i := 0; i < a.jsSize(); i++ {
		k, v := a.jsEntry(i)
		if deepIsObject(k) {
			pending = append(pending, k)
			continue
		}
		other := b.jsGet(k)
		if (other.kind != KindUndefined || b.jsHas(k)) && deepInnerEqual(v, other, mode, memos) {
			continue
		}
		if strict {
			return false
		}
		if !deepMapMightHaveLoosePrim(a, b, k, v, memos) {
			return false
		}
		pending = append(pending, k)
	}
	if pending == nil {
		return true
	}
	for i := 0; i < b.jsSize(); i++ {
		k, v := b.jsEntry(i)
		if deepIsObject(k) {
			var ok bool
			if pending, ok = deepTakeEqualEntry(pending, a, k, v, mode, memos); !ok {
				return false
			}
			continue
		}
		if strict {
			continue
		}
		if a.jsHas(k) && deepInnerEqual(a.jsGet(k), v, deepLoose, memos) {
			continue
		}
		var ok bool
		if pending, ok = deepTakeEqualEntry(pending, a, k, v, deepLoose, memos); !ok {
			return false
		}
	}
	return len(pending) == 0
}

// deepTakeEqual is node's setHasEqualElement: it finds a filed member deeply equal to
// v, removes it, and reports the pairing. Removing is what makes the two sides pair
// off one for one rather than every member of one matching a single member of the
// other.
func deepTakeEqual(pending []Value, v Value, mode deepMode, memos *deepMemos) ([]Value, bool) {
	for i, p := range pending {
		if deepInnerEqual(v, p, mode, memos) {
			return append(pending[:i], pending[i+1:]...), true
		}
	}
	return pending, false
}

// deepTakeEqualEntry is node's mapHasEqualEntry: the key must match a filed key
// deeply and the value stored under that filed key must match too, so two entries with
// equal keys and different values are not a pairing.
func deepTakeEqualEntry(pending []Value, a mapBacking, k, v Value, mode deepMode, memos *deepMemos) ([]Value, bool) {
	for i, p := range pending {
		if deepInnerEqual(k, p, mode, memos) && deepInnerEqual(v, a.jsGet(p), mode, memos) {
			return append(pending[:i], pending[i+1:]...), true
		}
	}
	return pending, false
}

// deepLoosePrimAlt is node's findLooseMatchingPrimitives, the question of whether a
// primitive could have a loosely equal partner that is not itself. It answers
// decided=true with the answer for the primitives that settle it outright: a symbol
// matches nothing but itself, and so does NaN, whether it arrived as a number or as a
// string that coerces to one, while every other number, string, boolean and bigint
// could have one. undefined and null are the undecided pair: each is loosely equal to
// the other and to nothing else, so the caller probes the collections for that partner,
// which alt names.
func deepLoosePrimAlt(v Value) (alt Value, answer, decided bool) {
	switch v.kind {
	case KindUndefined:
		return Null, false, false
	case KindNull:
		return Undefined, false, false
	case KindSymbol:
		return Undefined, false, true
	case KindString:
		if math.IsNaN(ToNumber(v)) {
			return Undefined, false, true
		}
	case KindNumber:
		if math.IsNaN(v.AsNumber()) {
			return Undefined, false, true
		}
	}
	return Undefined, true, true
}

// deepSetMightHaveLoosePrim is node's setMightHaveLoosePrim. For the undecided pair it
// is a real test rather than a guess: undefined can only match through the second set
// holding null while the first does not, since a null both sets hold has already been
// paired by its own direct lookup.
func deepSetMightHaveLoosePrim(a, b setBacking, prim Value) bool {
	alt, answer, decided := deepLoosePrimAlt(prim)
	if decided {
		return answer
	}
	return b.jsHas(alt) && !a.jsHas(alt)
}

// deepMapMightHaveLoosePrim is node's mapMightHaveLoosePrim, the map form of the
// probe: the partner key has to be in the second map with a value loosely equal to
// this entry's, and must not be in the first map, which would have paired it already.
func deepMapMightHaveLoosePrim(a, b mapBacking, prim, item Value, memos *deepMemos) bool {
	alt, answer, decided := deepLoosePrimAlt(prim)
	if decided {
		return answer
	}
	other := b.jsGet(alt)
	if (other.kind == KindUndefined && !b.jsHas(alt)) || !deepInnerEqual(item, other, deepLoose, memos) {
		return false
	}
	return !a.jsHas(alt)
}
