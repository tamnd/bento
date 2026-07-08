let x = 0;
const r = (x = 5);
console.log(r, x);

let i = 0;
const n = 3;
while ((i = i + 1) < n) {
  console.log(i);
}
console.log("done", i);

const a = [0, 0];
let y = 0;
a[0] = (y = 7);
console.log(a[0], y);
