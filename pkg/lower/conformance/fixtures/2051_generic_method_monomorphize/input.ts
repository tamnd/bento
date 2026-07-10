// A Go method carries no type parameter, so a generic method has no single Go
// form: bento emits one mangled Go method per concrete instantiation its call
// sites fix it to, keyed by the receiver type and the type argument. box.wrap(5)
// resolves against Wrap_num typed func(float64) float64, box.wrap(label) against
// Wrap_str typed func(value.BStr) value.BStr. The bare type parameter T in the
// body resolves to the concrete type each specialization was fixed to, so the
// return reads the same static type the call passes in.
class Box {
  wrap<T>(x: T): T {
    return x;
  }

  // a method that names T twice, in a parameter and the return, monomorphizes on
  // the same rule: pair(3, 4) fixes T to number and returns the first argument.
  pair<T>(a: T, b: T): T {
    return a;
  }
}

const b = new Box();
const label: string = "hi";
console.log(b.wrap(5));
console.log(b.wrap(label));
console.log(b.pair(3, 4));
console.log(b.pair(true, false));
