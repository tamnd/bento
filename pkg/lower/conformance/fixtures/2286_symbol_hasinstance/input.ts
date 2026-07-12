// A class can override instanceof by defining a static Symbol.hasInstance method the
// operator calls with the left operand. Honoring that needs the method installed
// under the well-known symbol key and instanceof routed through it rather than the
// prototype-chain walk. bento installs a class method under a constant string name,
// so a static member whose name is the Symbol.hasInstance computed key hands the class
// back rather than drop the hook and leave instanceof walking the default chain.
// Routing instanceof through a user Symbol.hasInstance is a later slice.
class Even {
  static [Symbol.hasInstance](value: unknown): boolean {
    return typeof value === "number" && value % 2 === 0;
  }
}
const four: unknown = 4;
console.log(four instanceof Even);
