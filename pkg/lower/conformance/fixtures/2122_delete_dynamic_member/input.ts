const obj: any = { a: 1, b: 2 };
const gone: boolean = delete obj.a;
console.log(gone);
console.log(obj.a);
console.log(obj.b);
