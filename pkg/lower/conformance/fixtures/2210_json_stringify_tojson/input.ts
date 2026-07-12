class Money {
  amount: number;
  constructor(a: number) { this.amount = a; }
  toJSON(): string { return "$" + this.amount; }
}
const m = new Money(5);
console.log(JSON.stringify(m));
const wrap = { label: "price", cost: m };
console.log(JSON.stringify(wrap));
console.log(JSON.stringify([m, m], null, 2));
