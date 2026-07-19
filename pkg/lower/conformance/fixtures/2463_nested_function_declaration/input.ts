// A function declaration nested inside another function's body is a helper the
// routine declares for its own use. JavaScript hoists it to the top of the
// enclosing scope, but as long as every call sits after the declaration the Go
// lowering can bind it to a local closure at its textual position and call that
// local. A recursive helper reads its own name, so the closure binds through the
// var-first two-step Go needs for a self-referential func value.

function classify(n: number): string {
  // A plain helper called by a later sibling statement.
  function label(x: number): string {
    return x < 0 ? "negative" : "nonnegative";
  }
  // A recursive helper, bound var-first so its body can call itself.
  function stepsToZero(x: number): number {
    if (x === 0) {
      return 0;
    }
    return 1 + stepsToZero(x - 1);
  }
  return label(n) + " in " + stepsToZero(n < 0 ? -n : n) + " steps";
}

console.log(classify(3));
console.log(classify(-2));
