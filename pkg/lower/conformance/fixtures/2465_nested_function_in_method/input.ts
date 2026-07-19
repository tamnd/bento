// A function declaration nested in a class method body binds to a Go local closure
// the same way one nested in a top-level function does. The method's parameters are
// the enclosing set the closure's Go name is vetted against, and a nested helper
// that does not read the method's this captures the ordinary way. A helper that
// reads its own name binds var-first.

class Grader {
  base: number;

  constructor(base: number) {
    this.base = base;
  }

  grade(score: number): string {
    // A plain helper called by a later statement in the method body.
    function bump(x: number): number {
      return x + 5;
    }
    // A recursive helper, bound var-first.
    function digits(x: number): number {
      return x < 10 ? 1 : 1 + digits((x - (x % 10)) / 10);
    }
    const raised = bump(score) + this.base;
    return raised + " has " + digits(raised) + " digits";
  }

  static passes(score: number): boolean {
    function threshold(): number {
      return 60;
    }
    return score >= threshold();
  }
}

const g = new Grader(10);
console.log(g.grade(90));
console.log(Grader.passes(45));
console.log(Grader.passes(72));
