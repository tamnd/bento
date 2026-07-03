// Package mapfixture is a go: import fixture for the section 6.5 map crossing. It
// models the two shapes a real library uses a map for: a producer that returns a
// map[string]int computed from its input, and a consumer that reads a map[string]int
// back. Counts proves a Go map result crosses into a bento Map entry by entry, and
// Total proves a bento Map argument crosses into a Go map the callee reads, so a
// round trip through the two is observable back on the bento side.
package mapfixture

import "strings"

// Counts returns the number of times each field of s appears, splitting on
// whitespace. The map[string]int result crosses back to a bento Map with a string
// key and a number value, so a bento program that reads a key sees the count the Go
// side computed.
func Counts(s string) map[string]int {
	out := map[string]int{}
	for _, w := range strings.Fields(s) {
		out[w]++
	}
	return out
}

// Total sums the values of m, so a bento Map handed in as a map[string]int argument
// is observable as the sum the Go side read back. It ignores the keys, which proves
// only that every value crossed in, not any particular order.
func Total(m map[string]int) int {
	sum := 0
	for _, v := range m {
		sum += v
	}
	return sum
}
