class Seq {
  *values(): Generator<number> {
    for (let i = 0; i < 3; i++) {
      yield i;
    }
  }
}
new Seq();
