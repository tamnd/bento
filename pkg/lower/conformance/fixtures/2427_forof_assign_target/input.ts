// A for...of head with no declaration assigns each element to an existing binding,
// `for (x of it)`, rather than declaring a fresh loop variable. When the target is a
// plain identifier bound to a widened primitive local whose Go representation already
// matches the element's, the loop ranges the same backing slice a declared binding
// ranges and assigns each element to the target at the top of the body. This covers a
// number array, a string array, string code points, a boolean array (whose element
// type the checker spells true | false yet lowers to a plain Go bool), and a numeric
// typed array, plus a target whose last assigned element survives the loop.
function sumInto(xs: number[]): number {
  let x = 0;
  let total = 0;
  for (x of xs) {
    total = total + x;
  }
  // The loop variable keeps the last element it was assigned.
  return total + x;
}

function joinInto(parts: string[]): string {
  let s = "";
  let out = "";
  for (s of parts) {
    out = out + s + ".";
  }
  return out;
}

function countTruthy(flags: boolean[]): number {
  let b = false;
  let n = 0;
  for (b of flags) {
    if (b) {
      n = n + 1;
    }
  }
  return n;
}

function codePoints(text: string): string {
  let c = "";
  let out = "";
  for (c of text) {
    out = out + c + "-";
  }
  return out;
}

function sumTyped(view: Float64Array): number {
  let v = 0;
  let total = 0;
  for (v of view) {
    total = total + v;
  }
  return total;
}

console.log(sumInto([1, 2, 3, 4]));
console.log(joinInto(["a", "b", "c"]));
console.log(countTruthy([true, false, true, true]));
console.log(codePoints("hé!"));
console.log(sumTyped(new Float64Array([1.5, 2.5, 3])));
