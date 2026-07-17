const pairs: [number, string][] = [[1, "a"], [2, "b"], [3, "c"]];
let a = 0;
let b = "";
for ([a, b] of pairs) {
  console.log(b + a);
}
console.log("last " + a + b);
