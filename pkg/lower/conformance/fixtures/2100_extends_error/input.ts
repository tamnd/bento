// A class that extends the built-in Error carries the inherited message and name
// as its own fields: super(message) fills the message, the name defaults to
// "Error" the way the built-in constructor sets it, and a this.name in the body
// overrides it. The subclass is throwable and catchable, and an instance answers
// instanceof Error, the shape a custom error class in real Node code takes.
class AppError extends Error {
  code: number;
  constructor(message: string, code: number) {
    super(message);
    this.name = "AppError";
    this.code = code;
  }
}

const e = new AppError("boom", 42);
console.log(e.message);
console.log(e.name);
console.log(e.code);
console.log(e instanceof Error);

try {
  throw new AppError("kapow", 7);
} catch (err: any) {
  console.log(err.name + " " + err.message);
}
