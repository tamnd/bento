// A ternary whose whole-expression type is a two-member T | undefined optional does
// not need the tagged sum: it is the value.Opt a T | undefined binding already holds.
// The present branch wraps as value.Some[T] and the undefined branch as value.None[T],
// so the result is the same Opt the slot's declared type spells, and a later presence
// test and narrowed read unwrap it through the ordinary optional path. Here `yes` takes
// the present branch and `no` takes the undefined branch, and each result is read back
// through a `!== undefined` guard, printing the value when present and a fallback when
// absent.
const yes = 1 > 0;
const no = 1 < 0;

const a: string | undefined = yes ? "present" : undefined;
const b: string | undefined = no ? "present" : undefined;

console.log(a !== undefined ? a : "absent");
console.log(b !== undefined ? b : "absent");
