// reduceRight folds right to left. With an initial value the accumulator starts
// there and visits elements from last to first, so a subtraction fold and a
// string fold both expose the order. The no-init form seeds the accumulator from
// the last element and folds toward the first, and an empty array with no initial
// value throws the same TypeError as reduce.
function subRight(a: number[]): number {
  return a.reduceRight((acc, n) => acc - n, 0);
}

function spellRight(a: number[]): string {
  return a.reduceRight((acc: string, n) => acc + n, "");
}

function subRightNoInit(a: number[]): number {
  return a.reduceRight((acc, n) => acc - n);
}

function run(): void {
  console.log(subRight([1, 2, 3]));
  console.log(spellRight([1, 2, 3]));
  console.log(subRightNoInit([1, 2, 3, 10]));
  console.log(subRightNoInit([42]));
}

run();
