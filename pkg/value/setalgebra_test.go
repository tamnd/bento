package value

import "testing"

// setOf builds a number set from the given members in order, the fixture the
// algebra tests share.
func setOf(members ...float64) *Set[float64] {
	s := NewNumberSet()
	for _, m := range members {
		s.Add(m)
	}
	return s
}

// order returns a set's members in insertion order, so a test can assert both the
// membership and the order the specification fixes for each algebra result.
func order(s *Set[float64]) []float64 {
	var out []float64
	s.Range(func(v float64) { out = append(out, v) })
	return out
}

func sameOrder(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSetUnion proves Union lists this set's members first, then the other's new
// members, and leaves both operands unchanged.
func TestSetUnion(t *testing.T) {
	a := setOf(1, 2, 3)
	b := setOf(3, 4, 5)
	got := order(a.Union(b))
	want := []float64{1, 2, 3, 4, 5}
	if !sameOrder(got, want) {
		t.Fatalf("union order = %v, want %v", got, want)
	}
	if !sameOrder(order(a), []float64{1, 2, 3}) || !sameOrder(order(b), []float64{3, 4, 5}) {
		t.Fatal("union mutated an operand")
	}
}

// TestSetIntersectionSmallerFirst proves Intersection follows the smaller set's
// order, this set's order when it is the smaller.
func TestSetIntersectionSmallerFirst(t *testing.T) {
	a := setOf(3, 1, 2)
	b := setOf(1, 2, 3, 4, 5)
	got := order(a.Intersection(b))
	want := []float64{3, 1, 2}
	if !sameOrder(got, want) {
		t.Fatalf("intersection order = %v, want %v", got, want)
	}
}

// TestSetIntersectionLargerFirst proves Intersection follows the other set's order
// when this set is the larger of the two.
func TestSetIntersectionLargerFirst(t *testing.T) {
	a := setOf(1, 2, 3, 4, 5)
	b := setOf(4, 2)
	got := order(a.Intersection(b))
	want := []float64{4, 2}
	if !sameOrder(got, want) {
		t.Fatalf("intersection order = %v, want %v", got, want)
	}
}

// TestSetDifference proves Difference keeps this set's members the other lacks, in
// this set's order.
func TestSetDifference(t *testing.T) {
	a := setOf(1, 2, 3, 4)
	b := setOf(2, 4, 6)
	got := order(a.Difference(b))
	want := []float64{1, 3}
	if !sameOrder(got, want) {
		t.Fatalf("difference order = %v, want %v", got, want)
	}
}

// TestSetSymmetricDifference proves SymmetricDifference lists this set's
// exclusive members first, then the other's, each in its own order.
func TestSetSymmetricDifference(t *testing.T) {
	a := setOf(1, 2, 3)
	b := setOf(3, 4, 5)
	got := order(a.SymmetricDifference(b))
	want := []float64{1, 2, 4, 5}
	if !sameOrder(got, want) {
		t.Fatalf("symmetric difference order = %v, want %v", got, want)
	}
}

// TestSetPredicates proves the three membership predicates over the subset,
// superset, and disjoint relations, including the equal-set edges.
func TestSetPredicates(t *testing.T) {
	a := setOf(1, 2)
	b := setOf(1, 2, 3)
	if !a.IsSubsetOf(b) {
		t.Fatal("expected a subset of b")
	}
	if b.IsSubsetOf(a) {
		t.Fatal("did not expect b subset of a")
	}
	if !b.IsSupersetOf(a) {
		t.Fatal("expected b superset of a")
	}
	if a.IsSupersetOf(b) {
		t.Fatal("did not expect a superset of b")
	}
	// a set is both a subset and a superset of itself
	self := setOf(7, 8)
	if !self.IsSubsetOf(setOf(7, 8)) || !self.IsSupersetOf(setOf(7, 8)) {
		t.Fatal("a set must be a subset and superset of an equal set")
	}
	if !a.IsDisjointFrom(setOf(4, 5)) {
		t.Fatal("expected a disjoint from {4,5}")
	}
	if a.IsDisjointFrom(b) {
		t.Fatal("did not expect a disjoint from b")
	}
}

// TestSetAlgebraNaNAndZero proves the algebra runs through SameValueZero: a NaN is
// a single member across sets and +0 and -0 are the same member.
func TestSetAlgebraNaNAndZero(t *testing.T) {
	nan := func() float64 { z := 0.0; return z / z }()
	a := setOf(nan, 0)
	b := setOf(nan, negZero())
	// both NaN and the zeroes match, so the sets are equal under SameValueZero
	if !a.IsSubsetOf(b) || !a.IsSupersetOf(b) {
		t.Fatal("expected NaN and signed zero to match across sets")
	}
	if got := order(a.Difference(b)); len(got) != 0 {
		t.Fatalf("expected an empty difference, got %v", got)
	}
}
