// A static generator method lowers to a receiver-less package function that returns the
// running coroutine, the same *value.Gen a top-level generator function returns; a static
// async generator lowers to the async coroutine, its *value.AsyncGen pulled by hand through
// await it.next(). The class name at the call site resolves to the class-prefixed function
// name, so Seq.upTo(4) drives the plain generator and Seq.doubles(3) the async one.
class Seq {
  static *upTo(n: number): Generator<number> {
    for (let i = 0; i < n; i++) {
      yield i;
    }
  }
  static async *doubles(n: number): AsyncGenerator<number> {
    for (let i = 0; i < n; i++) {
      yield i * 2;
    }
  }
}

let sum = 0;
for (const v of Seq.upTo(4)) {
  sum += v;
}
console.log(String(sum));

async function run(): Promise<void> {
  const it = Seq.doubles(3);
  let acc = 0;
  let r = await it.next();
  while (!r.done) {
    acc += r.value;
    r = await it.next();
  }
  console.log(String(acc));
}
run();
console.log("sync");
