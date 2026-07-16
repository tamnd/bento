// A binding declared with a non-optional static type and no initializer reads
// undefined in JavaScript until its first assignment, but a well-typed program under
// strict mode never observes that: the checker rejects any direct read before the
// binding is assigned as a use-before-assignment error, so every program that reaches
// the compiler has assigned the binding on every path that reaches a read. That makes
// the binding safe to lower to a plain Go var of the declared type, whose zero value
// stands in for the undefined the checker has already proven unobservable. This covers
// the shape across a number, a string, a boolean, and an array, each assigned on both
// arms of a branch and then read, plus a binding assigned in a nested block and one
// assigned but never read.
function classify(n: number): string {
  let label: string;
  if (n < 0) {
    label = "negative";
  } else if (n === 0) {
    label = "zero";
  } else {
    label = "positive";
  }
  return label;
}

function pickNumber(useFirst: boolean): number {
  let value: number;
  if (useFirst) {
    value = 10;
  } else {
    value = 20;
  }
  return value + 1;
}

function toFlag(b: boolean): boolean {
  let flag: boolean;
  flag = !b;
  return flag;
}

function firstOrZero(pick: boolean): number {
  let items: number[];
  if (pick) {
    items = [3, 6, 9];
  } else {
    items = [0];
  }
  // A nested block assigns a fresh binding the outer flow then reads.
  let total: number;
  {
    total = items.length;
  }
  return total;
}

// A binding assigned but never read is still declared; it must not trip the Go
// unused-variable rule.
function assignOnly(): void {
  let scratch: number;
  scratch = 42;
}

console.log(classify(-5));
console.log(classify(0));
console.log(classify(7));
console.log(pickNumber(true));
console.log(pickNumber(false));
console.log(toFlag(true));
console.log(firstOrZero(true));
console.log(firstOrZero(false));
assignOnly();
console.log("done");
