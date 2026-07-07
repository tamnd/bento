// BigInt division and remainder by zero throw a RangeError in JavaScript, a value a
// program can catch, where big.Int.Quo and Rem panic with a Go runtime error that no
// try can reach. So / and % lower through a value helper that checks the divisor and
// throws value.NewRangeError, which the uncaught reporter prints as a clean RangeError
// and exits non-zero rather than dumping a goroutine stack. The divide runs first, so
// the second line never prints. The test262 bigint division tests reach this to name
// the divide-by-zero boundary.
function divide(): void {
  const q: bigint = 1n / 0n;
  console.log(String(q));
}
console.log("before");
divide();
console.log("after");
