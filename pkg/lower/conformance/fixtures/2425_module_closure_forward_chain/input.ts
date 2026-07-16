// The forward-reference hoist is transitive. A top-level function reads the first
// closure, which forces it to a package var; that closure names a second module
// binding, whose own closure names a third, and each must hoist in turn so the chain
// of package vars resolves. Every reference points forward to a binding declared
// later, all legal because the closures run only after the module finishes
// initializing. The reads settle to 7 and the doubling helper reads the shared
// package var, not a stale main local.
const get = () => mid();
const mid = () => later;
const later = 7;

let calls = 0;

function run(): number {
  calls = calls + 1;
  return get();
}

console.log(run());
console.log(run());
console.log(calls);
