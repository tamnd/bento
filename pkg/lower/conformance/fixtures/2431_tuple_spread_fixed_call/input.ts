// A spread of a fixed-length tuple into a call whose callee has only fixed parameters
// expands to the tuple's element reads, f(...triple) becoming f(triple.E0, triple.E1,
// triple.E2), the same positional struct fields a tuple element access reads. The spread
// splices its members in position with no runtime array, so a required number tuple, a
// string tuple, and a spread after a leading positional argument all lower. The receiver
// must be repeatable, a binding whose element reads run no side effect twice, so a
// side-effecting spread source keeps the honest handback; this fixture drives only the
// provable-safe cases.

function add3(a: number, b: number, c: number): number {
  return a + b + c;
}

function join2(a: string, b: string): string {
  return a + b;
}

function tag(label: string, x: number, y: number): string {
  return label + ":" + (x + y);
}

const nums: [number, number, number] = [1, 2, 3];
const strs: [string, string] = ["he", "llo"];
const pair: [number, number] = [4, 5];

// A required number tuple spreads onto three parameters.
console.log(add3(...nums));
// A string tuple spreads onto two string parameters.
console.log(join2(...strs));
// A leading positional argument keeps its place ahead of the spread.
console.log(tag("sum", ...pair));
