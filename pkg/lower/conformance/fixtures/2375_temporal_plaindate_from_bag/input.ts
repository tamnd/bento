// PlainDate.from over a property bag builds a date from year, month, and day. The overflow rule
// defaults to constrain, so a day past the month's length clamps to the last day and a month past
// 12 clamps to December. A calendar in the bag interprets the year in that calendar's reckoning, so
// a roc year maps back to the ISO year the date stores, and the result carries the calendar.
console.log(Temporal.PlainDate.from({ year: 2020, month: 3, day: 14 }).toString());
console.log(Temporal.PlainDate.from({ year: 2020, month: 2, day: 31 }).toString());
console.log(Temporal.PlainDate.from({ year: 2021, month: 2, day: 31 }).toString());
console.log(Temporal.PlainDate.from({ year: 2020, month: 13, day: 5 }).toString());

const roc = Temporal.PlainDate.from({ year: 109, month: 5, day: 15, calendar: "roc" });
console.log(roc.toString());
console.log(roc.year);
console.log(roc.calendarId);

console.log(Temporal.PlainDate.from({ year: 2020, month: 5, day: 15, calendar: "gregory" }).toString());

try {
  Temporal.PlainDate.from({ year: 2020, month: 2, day: 31 }, { overflow: "reject" });
} catch (thrown: any) {
  console.log(thrown.constructor.name);
}
