// A user object can control its own coercion by defining a Symbol.toPrimitive method
// the ToPrimitive protocol calls with a hint. Honoring that hook needs the method
// installed on the object and the coercion sites, template interpolation and unary
// plus here, dispatched through it at run time. bento now boxes a plain parameterless
// coercion method into a live object (#504), but this hook reads its hint parameter,
// and a boxed method with a declared parameter needs the receiver-bound argument
// binding that free closure does not build, so the whole unit still hands back rather
// than drop the parameter and coerce through the default protocol. That parameter-
// carrying boxed method is a later slice; a plain object with no such hook coerces
// through the default already.
const money = {
  [Symbol.toPrimitive](hint: string): string | number {
    return hint === "number" ? 42 : "money";
  },
};
console.log(`${money}`);
console.log(+money);
