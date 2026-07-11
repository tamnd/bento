const proto: any = { a: 1 };
const child: any = Object.create(proto);
console.log(Object.getPrototypeOf(child) === proto);
const bare: any = Object.create(null);
console.log(Object.getPrototypeOf(bare) === null);
