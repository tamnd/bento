// Optional element access a?.[i] is the bracketed spelling of the optional chain: it
// reads the index only when the receiver is present and answers undefined otherwise,
// and it never evaluates the index expression on a nullish receiver. The shapes below
// cover the receivers the emitter tells apart. A regexp match is the everyday one, an
// array or string or object behind a T | undefined is the annotated one, a tuple reads
// a fixed position, and an any-typed receiver decides all of it at run time. A receiver
// the checker settled at present reads straight through, and one it settled at nullish
// folds to undefined without a test.

const m = /a(b)/.exec("xaby");
console.log(m?.[1]);
const none = /zz/.exec("xaby");
console.log(none?.[1]);

// The same read with no binding in between, which is how the line that stops a third of
// the compat suite is written. The checker types the call RegExpExecArray | null while
// the runtime answers one boxed value, so what decides the read is what the call lowers
// to rather than what the checker calls it.
const line = "Hardware: BCM2835";
console.log(/Hardware\s*:\s*(.*)/.exec(line)?.[1] === "BCM2835");
console.log(/Nothing\s*:\s*(.*)/.exec(line)?.[1] === "BCM2835");

let arr: number[] | undefined = [1, 2];
console.log(arr?.[1]);
arr = undefined;
console.log(arr?.[1]);

let s: string | undefined = "hi";
console.log(s?.[0]);
s = undefined;
console.log(s?.[0]);

const shape: { k: number } | undefined = { k: 7 };
console.log(shape?.["k"]);

function tuple(t: [number, string] | undefined): void {
  console.log(t?.[1]);
}
tuple([1, "z"]);
tuple(undefined);

function nth(a: number[] | undefined, i: number): number | undefined {
  return a?.[i];
}
console.log(nth([4, 5], 1), nth(undefined, 1));

const o: any = { rows: [{ k: 1 }], m: { a: 5 } };
const key = "a";
console.log(o.m?.[key]);
console.log(o.rows?.[0]?.k);
console.log(o.rows?.[0]?.missing);
console.log(o.nope?.[0]);

// The index is not evaluated when the receiver is nullish, so the counter only moves
// on the present receiver.
let evals = 0;
function idx(): number {
  evals++;
  return 0;
}
const gone: any = undefined;
console.log(gone?.[idx()]);
console.log("evals", evals);
console.log(o.rows?.[idx()]?.k);
console.log("evals", evals);
