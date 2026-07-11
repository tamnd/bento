const o: any = { a: 1, b: 2 };
console.log(delete o.a);
console.log(o.a);
console.log(delete o["b"]);
console.log(o.b);
console.log(delete o.missing);
