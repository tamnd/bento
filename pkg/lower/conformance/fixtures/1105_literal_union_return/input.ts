// A function whose return type is a union of literals that all share one primitive
// facet widens to that primitive: a numeric-literal union like 1 | 2 | 3 is a
// number, so it lowers to a float64 result, and true | false is a boolean, so it
// lowers to a bool. This is the same widening TypeScript applies, and it keeps the
// function off the tagged-sum path a general union of unlike arms needs (1101),
// which has no representation for a bare literal union and used to crash the printer.
// A closed string-literal union is deliberately not covered here: it lowers to a
// compact integer tag enum, a separate slice.
function rank(n: number): 1 | 2 | 3 {
  if (n > 5) return 3;
  if (n > 0) return 2;
  return 1;
}

function positive(n: number): true | false {
  return n > 0;
}

function run(): void {
  console.log(rank(7));
  console.log(rank(3));
  console.log(rank(-1));
  console.log(positive(3));
  console.log(positive(-1));
}

run();
