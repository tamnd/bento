// Package optfixture is a go: import fixture for the section 6.13 opaque handle
// crossing. It models the option-value shape a real library uses: a caller
// receives an option token from one call and hands it to another without ever
// inspecting it, which is exactly how a type the bridge does not project is meant
// to be used. Level is a struct with no exported fields and no exported methods, so
// it has no useful TypeScript projection and crosses as an opaque token bento holds
// and passes back.
package optfixture

// Level is an opaque option token. Its only field is unexported, so from bento it
// is a handle with no shape: the author receives one from WithLevel and hands it to
// Describe, never reading it.
type Level struct {
	n int
}

// WithLevel returns the option token that carries a level, the shape an
// option-value API hands the caller.
func WithLevel(n int) Level {
	return Level{n: n}
}

// Describe reads the level an option token carries and returns it, so a round trip
// through the opaque handle is observable back on the bento side as a number.
func Describe(opt Level) int {
	return opt.n
}
