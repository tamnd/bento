package value

import "testing"

// splitGo runs Split and renders the pieces as Go strings for comparison.
func splitGo(s string, sep string, limit ...float64) []string {
	arr := FromGoString(s).Split(FromGoString(sep), limit...)
	out := make([]string, len(arr.elems))
	for i, p := range arr.elems {
		out[i] = p.ToGoString()
	}
	return out
}

func eqStrs(a, b []string) bool {
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

// TestSplitLimit pins the optional limit argument against JavaScript's
// split(sep, limit): the result is truncated to the first limit pieces, a limit
// of zero yields the empty array, a limit past the piece count leaves the split
// whole, and ToUint32 coercion turns a negative or fractional limit into its
// unsigned truncation.
func TestSplitLimit(t *testing.T) {
	cases := []struct {
		s, sep string
		limit  []float64
		want   []string
	}{
		{"a,b,c,d", ",", []float64{2}, []string{"a", "b"}},
		{"a,b,c,d", ",", []float64{0}, nil},
		{"a,b,c,d", ",", []float64{10}, []string{"a", "b", "c", "d"}},
		{"a,b,c,d", ",", []float64{-1}, []string{"a", "b", "c", "d"}}, // ToUint32(-1) is huge
		{"a-b-c", "-", []float64{2.9}, []string{"a", "b"}},            // ToUint32 truncates toward zero
		{"abcd", "", []float64{2}, []string{"a", "b"}},                // empty separator, single units
		{"", "", []float64{3}, nil},                                  // empty string, empty sep is empty
		{"x,y", ",", nil, []string{"x", "y"}},                        // no limit is unbounded
	}
	for _, c := range cases {
		got := splitGo(c.s, c.sep, c.limit...)
		want := c.want
		if want == nil {
			want = []string{}
		}
		if !eqStrs(got, want) {
			t.Errorf("%q.split(%q, %v) = %v, want %v", c.s, c.sep, c.limit, got, want)
		}
	}
}
