// A default export can be an anonymous function declaration: a declaration with
// no name of its own. It still lowers to a top-level Go function, taking the
// synthesized Default name, and the rest of the program compiles and runs around
// it. The function need not be reachable in a single-file program for the
// declaration to lower cleanly.

export default function () {
  return 42;
}

console.log("ok");
