// Package funcfixture is a go: import fixture for the section 6.9 and 7.6 callback
// crossing. It models the shapes a real library uses a Go func parameter for: a
// helper that applies a callback once, one that folds a callback over a range to
// prove repeated calls, and a void callback invoked for its effect. Each proves a
// bento function crosses into a Go func value the callee calls, with the Go
// arguments marshaled to bento values and the bento result marshaled back to the Go
// return type.
package funcfixture

// Apply calls f with n once and returns what it produced, the smallest proof that a
// bento function crossed into a Go func(int) int the callee invokes and that its
// result crossed back to the Go int the callee returns.
func Apply(n int, f func(int) int) int {
	return f(n)
}

// SumTo folds f over 0..n-1 and returns the running total, so a callback that is
// called many times in a Go loop is observable as the sum on the bento side. It
// proves the wrapper survives repeated invocation, not a single call.
func SumTo(n int, f func(int) int) int {
	total := 0
	for i := range n {
		total += f(i)
	}
	return total
}

// Greet calls f with name and prefix and returns the greeting it built, proving a
// callback with more than one parameter, and mixed string and number parameters,
// marshals each argument by its own type across the boundary.
func Greet(name string, times int, f func(string, int) string) string {
	return f(name, times)
}

// Each calls f with every value in 0..n-1 for its effect and returns nothing, so a
// void callback (a Go func(int) with no result) is invoked once per value and its
// side effect is observable on the bento side.
func Each(n int, f func(int)) {
	for i := range n {
		f(i)
	}
}
