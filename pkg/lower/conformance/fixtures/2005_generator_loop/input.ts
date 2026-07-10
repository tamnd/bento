// A generator method whose body yields inside a loop lowers to the coroutine the
// same way a generator function does: the goroutine suspends at the yield mid-loop
// and resumes there on each pull, so a for...of over the method walks the loop one
// turn per value. This method yields the first three integers, and the loop prints
// them in order.
class Seq {
  *values(): Generator<number> {
    for (let i = 0; i < 3; i++) {
      yield i;
    }
  }
}

for (const v of new Seq().values()) {
  console.log(String(v));
}
