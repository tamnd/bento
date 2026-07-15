// withCalendar carries the three hosted non-ISO calendars onto a moved value, and the era-bearing
// getters re-derive off the ISO fields the receiver already holds.

const iso = Temporal.PlainDate.from("2024-03-15");

const roc = iso.withCalendar("roc");
console.log(roc.toString() + " " + roc.year + " " + roc.era + " " + roc.eraYear);

const jp = iso.withCalendar("japanese");
console.log(jp.toString() + " " + jp.year + " " + jp.era + " " + jp.eraYear);

// The era getter reads the moved value: a Japanese date one day before the Heisei era, moved forward
// a day, crosses the Showa-to-Heisei boundary and reports heisei 1.
const showa = Temporal.PlainDate.from({ year: 1989, month: 1, day: 7, calendar: "japanese" });
const heisei = showa.add({ days: 1 });
console.log(heisei.toString() + " " + heisei.era + " " + heisei.eraYear);

const dt = Temporal.PlainDateTime.from("2024-03-15T09:30:00");
const dtRoc = dt.withCalendar("roc");
console.log(dtRoc.toString() + " " + dtRoc.year + " " + dtRoc.era + " " + dtRoc.eraYear);

const z = Temporal.ZonedDateTime.from("2024-03-15T09:30:00-04:00[America/New_York]");
const zRoc = z.withCalendar("roc");
console.log(zRoc.year + " " + zRoc.era + " " + zRoc.eraYear + " " + zRoc.calendarId);
const zJp = z.withCalendar("japanese");
console.log(zJp.year + " " + zJp.era + " " + zJp.eraYear + " " + zJp.calendarId);
