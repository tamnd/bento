// A default parameter that reads an earlier parameter is evaluated in the callee
// scope, where that parameter is bound, not at the call site, which cannot see it.
// bento collapses the optional tail into one Go variadic and fills each optional in
// the body: a supplied argument lands in the variadic, an omitted one runs its
// default reading the earlier parameter. This proves the bare-read form end to end.
function pair(a: number, b: number = a): number {
  return a * 10 + b;
}

// b is omitted, so it defaults to a: 4 * 10 + 4 is 44.
console.log(pair(4));

// b is supplied, so the default does not run: 4 * 10 + 7 is 47.
console.log(pair(4, 7));
