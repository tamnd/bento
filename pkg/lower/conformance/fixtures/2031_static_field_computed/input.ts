// A static field whose initializer is not a plain constant runs in the class's
// static init function, in member order, so it can read an earlier static field
// or call a module function. The field declares its package var zero-valued and
// the initializer runs as an assignment where JavaScript evaluates it, which is
// at the class declaration's position in the main body.
function twice(n: number): number {
  return n * 2;
}

class Config {
  static base: number = 10;
  static doubled: number = twice(Config.base);
  static total: number = Config.base + Config.doubled;
  static { Config.total = Config.total + 1; }
}

console.log(String(Config.base));
console.log(String(Config.doubled));
console.log(String(Config.total));
