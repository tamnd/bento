const o: any = { a: 1 };
Object.preventExtensions(o);
o.b = 2;
console.log(o.b);
o.a = 5;
console.log(o.a);

const arr: any = [1, 2];
Object.preventExtensions(arr);
arr[5] = 9;
console.log(arr[5]);
arr[0] = 7;
console.log(arr[0]);
