package value

import "math"

// This file owns the numeric constants the Math and Number namespaces expose as
// properties: Math.PI and its siblings, and Number.MAX_SAFE_INTEGER and its. Each
// is the exact double the specification names, written as the shortest decimal
// that rounds to that double, so the compiler emits a reference to one of these in
// place of a Math.X or Number.X property read and the value matches the engine bit
// for bit. They live here rather than as inline literals in the lowerer so the
// generated Go names the constant it means (value.MathPI, not a bare 3.14...).

// The Math constants. Go's math package has most of these, but the derived ones
// (Log2E as 1/Ln2, for instance) are defined by a constant expression rather than
// a literal, so pinning the exact ECMAScript double here keeps them independent of
// how Go chose to spell its own.
const (
	MathE      = 2.718281828459045  // Math.E, Euler's number
	MathLN10   = 2.302585092994046  // Math.LN10, the natural log of 10
	MathLN2    = 0.6931471805599453 // Math.LN2, the natural log of 2
	MathLOG10E = 0.4342944819032518 // Math.LOG10E, the base-10 log of e
	MathLOG2E  = 1.4426950408889634 // Math.LOG2E, the base-2 log of e
	MathPI     = 3.141592653589793  // Math.PI
	MathSQRT12 = 0.7071067811865476 // Math.SQRT1_2, the square root of one half
	MathSQRT2  = 1.4142135623730951 // Math.SQRT2, the square root of two
)

// The finite Number constants.
const (
	NumberEpsilon        = 2.220446049250313e-16  // Number.EPSILON, 2^-52, the gap above 1
	NumberMaxSafeInteger = 9007199254740991       // Number.MAX_SAFE_INTEGER, 2^53 - 1
	NumberMinSafeInteger = -9007199254740991      // Number.MIN_SAFE_INTEGER
	NumberMaxValue       = 1.7976931348623157e308 // Number.MAX_VALUE, the largest finite double
	NumberMinValue       = 5e-324                 // Number.MIN_VALUE, the smallest positive subnormal
)

// The non-finite Number constants cannot be Go constants (an infinity or NaN is
// not a constant expression), so they are functions that build the value. The
// compiler emits the call in place of the property read.

// NumberPositiveInfinity is Number.POSITIVE_INFINITY.
func NumberPositiveInfinity() float64 { return math.Inf(1) }

// NumberNegativeInfinity is Number.NEGATIVE_INFINITY.
func NumberNegativeInfinity() float64 { return math.Inf(-1) }

// NumberNaN is Number.NaN.
func NumberNaN() float64 { return math.NaN() }
