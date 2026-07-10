// A static getter whose Go name collides with a static method the module
// already speaks: the method X mints CX, and the getter x would mint the same
// CX, so lowering hands back rather than give one Go name two meanings.
class C {
  static _x: number = 1;
  static X(): number {
    return 2;
  }
  static get x(): number {
    return C._x;
  }
}

console.log(String(C.x));
