// The typed-array callback methods run over the view with every element widened to
// a Number, so each callback takes the element as a number. map and filter build a
// fresh array of the same element kind, coercing each produced value the way an
// indexed write would; the fold methods reduce the view to a single value.
const a = new Int32Array([1, 2, 3, 4, 5]);

// forEach runs the callback for its side effect, in order.
let sum = 0;
a.forEach((x) => {
  sum += x;
});
console.log(sum);

// map returns a fresh typed array of the same kind, coercing each result.
const doubled = a.map((x) => x * 2);
console.log(doubled.join(","));

// a produced value is truncated into the element type, matching an indexed write.
const halved = a.map((x) => x / 2);
console.log(halved.join(","));

// filter keeps the elements the predicate accepts.
const evens = a.filter((x) => x % 2 === 0);
console.log(evens.join(","));

// some and every short-circuit; every over a passing predicate is true.
console.log(a.some((x) => x > 4));
console.log(a.every((x) => x > 0));
console.log(a.every((x) => x > 3));

// find and findIndex return the first match, findLast and findLastIndex the last.
console.log(a.find((x) => x > 2) ?? 0);
console.log(a.findIndex((x) => x > 2));
console.log(a.findLast((x) => x > 2) ?? 0);
console.log(a.findLastIndex((x) => x > 2));

// no match returns undefined from find and -1 from findIndex.
console.log(a.find((x) => x > 9) ?? 0);
console.log(a.findIndex((x) => x > 9));

// reduce and reduceRight with an initial value fold to an accumulator.
console.log(a.reduce((acc, x) => acc + x, 100));
console.log(a.reduceRight((acc, x) => acc + x, 0));

// the accumulator type may differ from the element Number.
console.log(a.reduce((acc, x) => acc && x > 0, true));

// with no initial value the fold seeds from an end element.
console.log(a.reduce((acc, x) => acc + x));
console.log(a.reduceRight((acc, x) => acc - x));
