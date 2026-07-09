// A callable object whose name an earlier statement captures inside a closure
// must have its pointer declared before that statement, the shape the test262
// assert prelude writes: assert.compareArray = function () { ... compareArray ...
// } sits above const compareArray = function () { ... }. In JavaScript the const
// is scoped to the whole module, so the closure's forward reference is legal and
// only read when the closure later runs. Go has no forward capture, so the
// binding's pointer hoists to the scope top and its own site lowers to a plain
// assignment, leaving every alias sharing the one object.
interface Asserter {
  (ok: boolean): void;
  cmp: (a: number[], b: number[]) => void;
}
interface Comparer {
  (a: number[], b: number[]): boolean;
  format: (a: number[]) => string;
}
const check = function (ok: boolean): void {
  if (!ok) console.log("fail");
} as Asserter;
check.cmp = function (a: number[], b: number[]): void {
  const ok = compare(a, b);
  check(ok);
  console.log(compare.format(a));
};
const compare = function (a: number[], b: number[]): boolean {
  return a.length === b.length;
} as Comparer;
compare.format = function (a: number[]): string {
  return "[" + a.length + "]";
};
check.cmp([1, 2], [3, 4]);
check.cmp([1], [3, 4]);
