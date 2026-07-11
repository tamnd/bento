const o: any = { a: 1 };
Object.freeze(o);
const d: any = Object.getOwnPropertyDescriptor(o, "a");
console.log(d.writable);
console.log(d.configurable);
o.a = 9;
console.log(o.a);
o.b = 2;
console.log(o.b);

const arr: any = [1, 2];
Object.freeze(arr);
arr[0] = 9;
console.log(arr[0]);
