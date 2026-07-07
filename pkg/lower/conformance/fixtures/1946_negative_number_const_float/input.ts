// A negative numeric initializer is a unary minus wrapping the literal, so a const
// or let bound to one lowered to a Go int short declaration (n := -5) rather than a
// float64 (n := -5.0), because the retyping step only recognized a bare positive
// literal. Go infers int from -5, and passing that binding into a float64 parameter,
// value.Pow for ** and Math.pow, fails to build with "cannot use n (int) as float64".
// Peeling the sign off, retyping the inner literal, and putting the minus back keeps
// the binding a float64. A test262 exponentiation probe drives it with INT32_MIN as
// the exponent; a plain Math.pow and an ordinary addition confirm the common path.
const INT32_MIN = -2147483648;
console.log(2 ** INT32_MIN);
console.log(1 ** INT32_MIN);

const neg = -5;
console.log(Math.pow(2, neg));
console.log(neg + 1);

let e = -3;
console.log(Math.pow(2, e));
