// A user object can control its own coercion by defining a Symbol.toPrimitive method
// the ToPrimitive protocol calls with a hint. Honoring that hook needs the method
// installed on the object and the coercion sites, template interpolation and unary
// plus here, dispatched through it at run time. bento lowers a static object to a Go
// struct with no place to hang a symbol-keyed method, so an object literal carrying a
// method member hands the whole unit back rather than drop the toPrimitive hook and
// coerce through the default protocol. Routing ToPrimitive through a user method is a
// later slice; a plain object with no such hook coerces through the default already.
const money = {
  [Symbol.toPrimitive](hint: string): string | number {
    return hint === "number" ? 42 : "money";
  },
};
console.log(`${money}`);
console.log(+money);
