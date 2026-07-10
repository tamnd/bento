// A default parameter may read an earlier parameter through a call or an expression,
// not just a bare read. The callee-scope variadic fill lowers these the same way: the
// default runs in the body where the earlier parameters and earlier optionals are
// bound, so a call over the first parameter and an expression over the second both
// settle before the body reads them.
function twice(n: number): number {
  return n * 2;
}

function span(start: number, end: number = twice(start), step: number = end - start): number {
  return start + end + step;
}

// end defaults to twice(3) is 6, step defaults to 6 - 3 is 3: 3 + 6 + 3 is 12.
console.log(span(3));

// end is supplied as 10, step defaults to 10 - 3 is 7: 3 + 10 + 7 is 20.
console.log(span(3, 10));

// all supplied: 3 + 10 + 2 is 15.
console.log(span(3, 10, 2));
