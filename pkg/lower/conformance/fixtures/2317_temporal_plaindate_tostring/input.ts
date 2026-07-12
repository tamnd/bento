// Temporal.PlainDate.prototype.toString and toJSON render the ISO 8601 date. A year
// in 0..9999 is four digits; a year outside that range takes the expanded signed
// six-digit form, so a negative year and a year past 9999 both round-trip through the
// leading sign. toJSON produces the same string toString does under default options.
const d = new Temporal.PlainDate(2020, 2, 29);
console.log(d.toString());
console.log(d.toJSON());
console.log(new Temporal.PlainDate(-1, 12, 31).toString());
console.log(new Temporal.PlainDate(275760, 9, 13).toString());
console.log(new Temporal.PlainDate(12345, 6, 7).toString());
