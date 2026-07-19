// A compound write o.k <op>= v on a dynamic receiver has no static field to update,
// so it loads the property through the boxed value's Get, runs the boxed arithmetic,
// and stores the result back through Set, the dotted mirror of the bracket compound
// o[k] <op>= v. A number property runs the numeric operator; a string property's +=
// concatenates through value.Add.
const o: any = { n: 10, s: "a" };
o.n += 5;
o.n *= 2;
o.n -= 1;
o.s += "b";
console.log(o.n);
console.log(o.s);
