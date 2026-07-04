// This file owns Math.random, the one Math method whose result is not a pure
// function of its arguments (06_math). Every other Math method computes the same
// number from the same inputs, so the differential oracle proves it by running the
// TypeScript and the lowered Go side by side and comparing output. Math.random
// cannot be proven that way: it returns a fresh number on every call and the two
// runtimes draw from unrelated generators, so their raw output never matches. Its
// conformance is checked by shape instead, that every draw lands in [0, 1) and that
// the draws are not all the same, which both runtimes satisfy.

package value

import "math/rand/v2"

// MathRandom is Math.random(): a float64 uniformly distributed in [0, 1). It draws
// from the math/rand/v2 top-level generator, which is seeded from the operating
// system at startup and is safe to call from any goroutine, so a lowered program
// needs no generator plumbing of its own. The [0, 1) range is exactly the range
// rand.Float64 and Math.random share, so no rescaling is needed.
func MathRandom() float64 { return rand.Float64() }
