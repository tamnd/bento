// A switch over a boolean is the case-chain idiom: switch (true) picks the first
// case whose test is true, the multi-way form of an if-else ladder. JavaScript
// matches each label with strict equality against the discriminant, so with a
// boolean discriminant and boolean labels it is Go's == and lowers to a Go switch
// on the bool. Bento handed a boolean-discriminant switch back as a later slice
// before, so this idiom never lowered.
function rank(n: number): string {
  switch (true) {
    case n > 10:
      return "big";
    case n > 5:
      return "mid";
    default:
      return "small";
  }
}
console.log(rank(12));
console.log(rank(7));
console.log(rank(2));

// A switch on a boolean variable matches by strict equality too, so a case whose
// label is a boolean test runs when it equals the discriminant.
function parity(n: number): string {
  const even = n % 2 === 0;
  switch (even) {
    case true:
      return "even";
    default:
      return "odd";
  }
}
console.log(parity(4));
console.log(parity(3));
