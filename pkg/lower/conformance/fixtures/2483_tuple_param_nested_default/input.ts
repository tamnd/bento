function f([[a, b], c]: [[number, string], boolean]): string {
  return a + ":" + b + ":" + c;
}
console.log(f([[1, "x"], true]));

function g([p, q = 99]: [number, number]): number {
  return p + q;
}
console.log(g([3, 4]) + "");

function h([{ name }, count]: [{ name: string }, number]): string {
  return name + count;
}
console.log(h([{ name: "ab" }, 2]));
