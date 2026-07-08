let n = 0;
const a = ++n;
const b = n++;
const c = n--;
const d = --n;
console.log(a, b, c, d, n);

const arr = [10, 20, 30];
let i = 0;
const first = arr[i++];
const second = arr[i++];
console.log(first, second, i);
