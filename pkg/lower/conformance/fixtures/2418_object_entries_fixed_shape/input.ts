const nums = { a: 1, b: 2, c: 3 };
for (const [k, v] of Object.entries(nums)) {
  console.log(k, v);
}
const e = Object.entries(nums);
console.log(e.length);
console.log(e[0][0], e[0][1]);
const words = { first: "hello", second: "world" };
for (const [k, v] of Object.entries(words)) {
  console.log(k, v);
}
