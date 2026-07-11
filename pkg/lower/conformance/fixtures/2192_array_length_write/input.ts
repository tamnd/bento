const a: any = [1, 2, 3];

// growing the length extends the array with holes: the new indices are absent own
// properties that read undefined, so Object.keys still lists only the first three.
a.length = 5;
console.log(a.length);
console.log(Object.keys(a).length);
console.log(a[4]);

// shrinking the length truncates the array, dropping the tail elements.
const b: any = [1, 2, 3, 4, 5];
b.length = 2;
console.log(b.length);
console.log(Object.keys(b).length);
