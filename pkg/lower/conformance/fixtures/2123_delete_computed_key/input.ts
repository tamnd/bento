const obj: any = { a: 1, b: 2 };
const g1: boolean = delete obj["a"];
console.log(g1);
console.log(obj.a);
const arr: any = [10, 20, 30];
const g2: boolean = delete arr[1];
console.log(g2);
console.log(arr[1]);
console.log(arr.length);
