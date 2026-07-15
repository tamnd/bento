// Temporal.Duration.prototype.add and subtract fold the receiver and a Duration operand over a
// fixed 24-hour day and balance the result to the coarser of the two operands' default largest
// units. The reduced Temporal profile drops the relativeTo option, so neither can balance the
// irregular calendar units: an operand that carries years, months, or weeks throws a RangeError.
// Every result was checked against @js-temporal/polyfill.
const twoDays = Temporal.Duration.from({ days: 2 });
console.log(twoDays.add(Temporal.Duration.from({ hours: 50 })).toString());
console.log(Temporal.Duration.from({ hours: 1, minutes: 30 }).add(Temporal.Duration.from({ minutes: 30 })).toString());
console.log(Temporal.Duration.from({ days: 2, hours: 3 }).subtract(Temporal.Duration.from({ hours: 5 })).toString());
console.log(Temporal.Duration.from({ minutes: 90 }).add(Temporal.Duration.from({ minutes: 90 })).toString());
console.log(Temporal.Duration.from({ days: 1 }).subtract(Temporal.Duration.from({ hours: 36 })).toString());

try {
  Temporal.Duration.from({ months: 1 }).add(Temporal.Duration.from({ days: 1 }));
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  Temporal.Duration.from({ days: 1 }).add(Temporal.Duration.from({ weeks: 1 }));
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}

try {
  Temporal.Duration.from({ years: 1 }).subtract(Temporal.Duration.from({ days: 1 }));
  console.log("no throw");
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}
