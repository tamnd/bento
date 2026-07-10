// A module-level binding a top-level function reads hoists to a package var. When
// the initializer is a literal it moves whole to package scope, but when it is a
// call or an expression over other module state it cannot run at package-init time.
// Such a binding becomes a zero-valued package var and its statement stays in main
// to assign it at its source position, so the module top-level runs in order and the
// function reads the settled value. This proves the in-place-assignment hoist end to
// end: a call initializer, an expression over an earlier binding, and a chain that
// reads the one before it.
function base(): number {
  return 10;
}

// A call initializer runs in main, not at package init.
const start = base();

// An expression over an earlier module binding.
const next = start + 5;

// A chain: this reads the binding declared just before it.
const total = next * 2;

function report(): number {
  return start + next + total;
}

// start is 10, next is 15, total is 30, so the sum is 55.
console.log(report());
