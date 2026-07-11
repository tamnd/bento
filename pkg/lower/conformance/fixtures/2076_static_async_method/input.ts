// A static async method returns a promise the same way an instance async method does,
// only with no receiver: it lowers to a package function that settles its promise. The
// resolved value reaches a .then callback after the synchronous run, at the microtask
// checkpoint, so it prints after the trailing line.
class Calc {
  static async of(v: number): Promise<number> {
    return v + 1;
  }
}

console.log("start");
Calc.of(41).then((r) => console.log(r));
console.log("end");
