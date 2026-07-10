// extends null is a base clause bento does not model: the class has no
// prototype chain to embed. It hands back with its own named reason rather than
// the generic heritage message the base-class split retired.
class Bare extends null {
  x: number = 1;
}

console.log(String(new Bare().x));
