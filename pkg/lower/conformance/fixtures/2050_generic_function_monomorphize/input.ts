// A generic function has no single Go form, so bento emits one specialized Go
// function per concrete type argument its call sites fix it to: identity(5)
// resolves against Identity_num typed func(float64) float64, identity("hi")
// against Identity_str typed func(value.BStr) value.BStr. The bare type
// parameter T in the body resolves to the concrete type each specialization was
// fixed to, so the return reads the same static type the call passes in.
function identity<T>(x: T): T {
  return x;
}

// two distinct instantiations, number and string.
console.log(identity(5));
console.log(identity("hi"));

// a second generic that names T twice, in a parameter and the return, fixed to
// boolean here, so it monomorphizes to its own specialization.
function firstOf<T>(a: T, b: T): T {
  return a;
}

console.log(firstOf(true, false));
console.log(firstOf(10, 20));
