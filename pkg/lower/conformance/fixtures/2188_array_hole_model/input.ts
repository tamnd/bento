const a: any = [1, 2, 3];
delete a[1];
console.log(a.length);
console.log(Object.keys(a).length);
console.log(a[1]);

const b: any = [1, undefined, 3];
console.log(Object.keys(b).length);

const c: any = [0];
c[3] = 9;
console.log(c.length);
console.log(Object.keys(c).length);

console.log(JSON.stringify(a));
