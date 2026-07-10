// An explicit undefined argument in a defaulted slot counts as a missing argument:
// the language fills the parameter's default for undefined exactly as it does for an
// omission. bento substitutes the default at the call site rather than lowering
// undefined into a slot whose static type cannot hold it. This proves the
// undefined-triggers-default rule end to end.
function inc(x: number, by: number = 1): number {
  return x + by;
}

// by is passed undefined, which counts as missing, so it defaults to 1: 5 + 1 is 6.
console.log(inc(5, undefined));

// by is supplied, so the default does not run: 5 + 3 is 8.
console.log(inc(5, 3));
