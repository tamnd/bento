// A write to arguments[i] stores into the backing snapshot the function
// materialized from its parameters, so a following read of arguments[i] sees the
// written value. The body reads no parameter by name, so the unmapped store rule
// (arguments does not alias the named parameters) is indistinguishable from the
// mapped rule and the write lowers instead of handing back.
function set(a: number, b: number): unknown {
  arguments[0] = 42;
  return arguments[0];
}

// the write replaced the first argument, so this reads 42.
console.log(set(1, 2));

function swapReturn(a: number, b: number): unknown {
  arguments[1] = arguments[0];
  return arguments[1];
}

// arguments[1] takes the value of arguments[0], 7.
console.log(swapReturn(7, 8));
