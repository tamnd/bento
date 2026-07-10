// TypeScript allows statements before super() in a derived constructor as long
// as they do not read this, which is in the temporal dead zone until super
// returns. The this-free leading statements lower before the base assignment,
// the order JavaScript runs them, and the super arguments feed the base
// constructor after them.
class Base {
  label: string;
  constructor(label: string) {
    this.label = label;
  }
}

class Derived extends Base {
  constructor(n: number) {
    console.log("preparing");
    console.log("n is " + String(n));
    super("d" + String(n * 2));
  }
}

const d = new Derived(5);
console.log(d.label);
