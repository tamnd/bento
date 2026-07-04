// A general union of two unrelated types has no lowering yet: it needs the tagged
// sum struct that section 9 of the plan describes, which is the highest-value
// pending type in the checklist. Until that lands, a function returning number |
// string hands its whole unit back to the interpreter rather than emitting Go that
// would have to guess a single Go type for a value that is one of two.
export function pick(b: boolean): number | string {
  return b ? 1 : "a";
}
