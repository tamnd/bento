// Arithmetic on number literals whose result runs past the float64 range folds
// to a Go constant the compiler rejects as overflowing, where JavaScript yields
// Infinity at runtime. A finite pair can only overflow to an infinity, so the
// expression lowers to that infinity directly and the generated Go compiles.
// The test262 number tests reach for this to name the boundary, computing
// 1e308 * 2 and 1e308 + 1e308 where Number.POSITIVE_INFINITY is expected.
const over = 1e308 * 2;
const under = -1e308 * 2;
const sum = 1e308 + 1e308;
console.log(String(over));
console.log(String(under));
console.log(String(sum));
