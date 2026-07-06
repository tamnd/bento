// A thrown class instance rides the runtime's panic path when the class
// carries a string message field: the class gains ErrorName and ErrorMessage
// in emission, so the catch binds an error named after the class and the
// message reads off the instance. test262's Test262Error throws this way.
class Boom {
  message: string;
  constructor(reason: string) {
    this.message = "boom: " + reason;
  }
}

function detonate(): never {
  throw new Boom("kapow");
}

try {
  detonate();
} catch (err: any) {
  console.log(err.name + " " + err.message);
}
