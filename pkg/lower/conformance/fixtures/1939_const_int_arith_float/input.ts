// Arithmetic on two integer-spelled number literals is float64 in JavaScript, but Go
// folds an integer-spelled constant as an int: 5 + 3 under := infers int, and 7 / 2
// folds to the integer 3 rather than 3.5. Each expression folds to its float64 value so
// the local reads as float64 like every JavaScript number and the division carries its
// real quotient. A chained divide reduces the same way, and a large integer-valued
// product keeps its full spelling. The test262 addition, subtraction, multiplication,
// and division number tests reach these shapes through the harness assertions.
const sum = 5 + 3;
const diff = 5 - 1;
const prod = 6 * 2;
const quot = 7 / 2;
const chain = 18 / 2 / 9;
const big = 1048576 * 1;
console.log(String(sum));
console.log(String(diff));
console.log(String(prod));
console.log(String(quot));
console.log(String(chain));
console.log(String(big));
