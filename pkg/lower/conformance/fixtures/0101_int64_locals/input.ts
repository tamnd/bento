// A number local whose every value is a whole number inside the safe-integer
// range, but wider than 32 bits, lowers to a Go int64. Doubles represent whole
// numbers exactly up to 2^53, so as long as the analysis proves the local never
// leaves that range the integer arithmetic and the double arithmetic agree bit
// for bit. There is no runtime coercion that could paper over a wrong proof, so
// a local the analysis cannot bound simply stays a double; the printed results
// here are the same either way, which is the point.
let sum = 0;
for (let i = 1; i <= 100000; i++) {
  sum = sum + i * i;
}
console.log(sum);

let neg = 0;
for (let j = 1; j <= 100000; j++) {
  neg = neg - j * j;
}
console.log(neg);

let big = 5000000000;
big = big + 1;
console.log(big);
console.log(sum + big);
