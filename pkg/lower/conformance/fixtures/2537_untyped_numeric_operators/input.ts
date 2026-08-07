// An arithmetic or bitwise operator whose operands do not agree on a numeric kind,
// a bigint against a number or against a value that could be either, has no static
// answer: 2n * 2n is 4n, 2 * 2 is 4, and 2n * 2 throws. The checker reports the
// disagreement by giving the expression no type at all, so the operator runs as the
// runtime's own version of itself and answers whichever kind it finds.
//
// Every line below is written the way node's own test harness writes it, over a
// number | bigint the checker cannot narrow. The // @ts-ignore comments are there
// because TypeScript reports the mix as an error, which is exactly the signal this
// lowering reads.

function show(name: string, f: () => unknown): void {
  try {
    console.log(name, f());
  } catch (e) {
    console.log(name, "threw", (e as Error).message);
  }
}

function pick(big: boolean): number | bigint {
  return big ? 6n : 6;
}

const b: number | bigint = pick(true);
const n: number | bigint = pick(false);

// The five arithmetic operators over a pair that turns out to be two bigints.
// @ts-ignore
show("big-plus", () => b + pick(true));
// @ts-ignore
show("big-minus", () => b - pick(true));
// @ts-ignore
show("big-times", () => b * pick(true));
// @ts-ignore
show("big-div", () => b / pick(true));
// @ts-ignore
show("big-rem", () => b % pick(true));
// @ts-ignore
show("big-pow", () => b ** pick(true));

// The same five over a pair that turns out to be two numbers. The operator is the
// same one; only what it is handed differs, which is the whole point of routing the
// decision to the runtime.
// @ts-ignore
show("num-plus", () => n + pick(false));
// @ts-ignore
show("num-minus", () => n - pick(false));
// @ts-ignore
show("num-times", () => n * pick(false));
// @ts-ignore
show("num-div", () => n / pick(false));
// @ts-ignore
show("num-rem", () => n % pick(false));
// @ts-ignore
show("num-pow", () => n ** pick(false));

// A pair that really is mixed throws, the error the language raises rather than
// coercing one side. + throws the same way, since a bigint and a number never add.
// @ts-ignore
show("mixed-times", () => b * pick(false));
// @ts-ignore
show("mixed-plus", () => b + pick(false));

// The bitwise operators over a bigint pair, which stay bigints: a bigint shift is
// arbitrary width, not the 32-bit wrap a number takes.
// @ts-ignore
show("big-and", () => b & pick(true));
// @ts-ignore
show("big-or", () => b | pick(true));
// @ts-ignore
show("big-xor", () => b ^ pick(true));
// @ts-ignore
show("big-shl", () => b << pick(true));
// @ts-ignore
show("big-shr", () => b >> pick(true));

// The unsigned right shift is the one operator with no bigint meaning at all, so a
// bigint pair throws where a number pair wraps to 32 bits.
// @ts-ignore
show("big-ushr", () => b >>> pick(true));
// @ts-ignore
show("num-and", () => n & pick(false));
// @ts-ignore
show("num-shl", () => n << pick(false));
// @ts-ignore
show("num-ushr", () => n >>> pick(false));

// A binding over such an operator holds the runtime's answer and reports its kind.
// @ts-ignore
const prod = b * pick(true);
console.log("bind-prod", prod);
console.log("bind-typeof", typeof prod);
// @ts-ignore
const sum = n + pick(false);
console.log("bind-sum", sum);
console.log("bind-sum-typeof", typeof sum);

// A chain of them keeps working: the inner operator's answer is what the outer one
// is handed.
// @ts-ignore
show("chain", () => b * pick(true) - pick(true));

// A function whose only return is such an operator hands back what the runtime
// computed, and a call of it flows on into a dynamic slot as itself.
// @ts-ignore
function mul(x: number | bigint, y: number | bigint) { return x * y; }
const z: unknown = mul(3n, 4n);
console.log("fn-big", z);
console.log("fn-num", mul(3, 4));
console.log("fn-typeof", typeof z);

// The shape node's test harness writes, which is where this arises in practice: a
// multiplier picked by the argument's own kind, multiplied back into it.
function platformTimeout(ms: number | bigint): number | bigint {
  const multipliers = typeof ms === "bigint" ? { two: 2n } : { two: 2 };
  // @ts-ignore
  return multipliers.two * ms;
}
console.log("timeout-num", platformTimeout(3));
console.log("timeout-big", platformTimeout(3n));
