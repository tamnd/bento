function foo() {}
foo.x = 1;
const a = Object.keys(foo);
console.log(a.length === 1);
console.log(a[0]);
