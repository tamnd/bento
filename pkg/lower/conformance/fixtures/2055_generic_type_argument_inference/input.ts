// A call fixes a generic's type argument by inference from the arguments it passes,
// not only from an explicit <T> the source spells. first<T> reaches T only through
// its parameter typed T[], so first([10, 20, 30]) infers T=number from the array's
// element type and instantiates First_num over *value.Array[float64], and first over
// a string[] infers T=string and instantiates a second specialization. Neither call
// writes a type argument; the monomorphizer reads it off the argument type the way
// the checker does.
function first<T>(xs: T[]): T {
  return xs[0];
}

console.log(first([10, 20, 30]));

const words: string[] = ["a", "b"];
console.log(first(words));
