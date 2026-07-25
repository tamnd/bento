// A user object can control its own coercion by defining a Symbol.toPrimitive method
// the ToPrimitive protocol calls with a hint. Honoring that hook needs the method
// installed on the object and the coercion sites, template interpolation and unary
// plus here, dispatched through it at run time. The object literal carries a
// symbol-keyed member, which belongs in the object side table an interned struct
// does not model, so the whole unit hands back at that key before it reaches the
// method's declared hint parameter or the coercion dispatch. A plain object with no
// such hook coerces through the default protocol already.
const money = {
  [Symbol.toPrimitive](hint: string): string | number {
    return hint === "number" ? 42 : "money";
  },
};
console.log(`${money}`);
console.log(+money);
