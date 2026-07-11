const entries: any = [["a", 1], ["b", 2], ["a", 9]];
const o: any = Object.fromEntries(entries);
console.log(o.a);
console.log(o.b);

const none: any = [];
const empty: any = Object.fromEntries(none);
console.log(empty.missing);
