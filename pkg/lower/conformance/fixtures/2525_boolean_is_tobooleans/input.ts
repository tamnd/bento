// Boolean(x) is the ToBoolean x undergoes standing in a boolean position, so it reads
// the same falsy set an if reads. The suite's own harness gates on the dynamic form,
// Boolean(process.versions.openssl), which is a property read off a value whose kind is
// only known at runtime, so that shape leads here.
const cfg = JSON.parse('{"zero":0,"empty":"","text":"x","nil":null,"nested":{}}');
console.log(Boolean(cfg.zero), Boolean(cfg.empty), Boolean(cfg.text));
console.log(Boolean(cfg.nil), Boolean(cfg.missing), Boolean(cfg.nested), Boolean(cfg));

// An argument the checker proves always truthy collapses to a constant, which drops the
// argument's read. A binding whose only read is that one has to keep a blank in the Go
// or the program would not compile, so both spellings of the fold are here.
const rows: number[] = [];
const opts = { deep: 1 };
console.log(Boolean(rows), Boolean(opts), !rows, !opts);

// A binding the fold drops one read of but the program reads again is still live.
console.log(Boolean(rows), rows.length);

// The primitives keep their own named tests: a number is falsy at zero and NaN, a
// string only when empty, so "0" is truthy, and a boolean is its own truth.
console.log(Boolean(1), Boolean(0), Boolean(0 / 0), Boolean(-1));
console.log(Boolean("x"), Boolean("0"), Boolean(""));
console.log(Boolean(true), Boolean(false));

// An optional is falsy two ways, absent or present and falsy.
function has(s?: string): boolean {
  return Boolean(s);
}
console.log(has("x"), has(""), has());
