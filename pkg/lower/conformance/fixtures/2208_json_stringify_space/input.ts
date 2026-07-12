const obj = { a: 1, b: [2, 3], c: { d: 4 } };
console.log(JSON.stringify(obj, null, 2));
const arr = [1, 2, 3];
console.log(JSON.stringify(arr, null, 2));
const tab = { a: 1, nested: { b: 2 } };
console.log(JSON.stringify(tab, null, "\t"));
const empty = {};
console.log(JSON.stringify(empty, null, 4));
