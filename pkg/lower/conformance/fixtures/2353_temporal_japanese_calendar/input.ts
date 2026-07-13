// The japanese calendar keeps the ISO year, month, and day, so only the era differs. A
// date-aware nengo table names the era and counts eraYear from the era's first ISO year:
// reiwa from 2019-05-01 (eraYear ISO-2018), heisei from 1989-01-08 (ISO-1988), showa from
// 1926-12-25 (ISO-1925), taisho from 1912-07-30 (ISO-1911), meiji from 1868-09-08
// (ISO-1867). The era turns on the whole date, so 1989-01-07 is still showa 64 while
// 1989-01-08 is heisei 1. Before meiji the era splits at ISO year 1 like gregory:
// "japanese" eraYear ISO year, "japanese-inverse" eraYear 1 minus the ISO year. toString
// prints the unchanged ISO year with the [u-ca=japanese] annotation. compare ignores the
// calendar; equals folds it in. Ids are case-insensitive.
const d = new Temporal.PlainDate(2024, 6, 30, "japanese");
console.log(d.calendarId);
console.log(d.year);
console.log(d.era);
console.log(d.eraYear);
console.log(d.month, d.day);
console.log(d.toString());

const heisei = new Temporal.PlainDate(1989, 1, 8, "japanese");
console.log(heisei.era, heisei.eraYear);

const showa = new Temporal.PlainDate(1989, 1, 7, "japanese");
console.log(showa.era, showa.eraYear);

const meiji = new Temporal.PlainDate(1868, 9, 8, "japanese");
console.log(meiji.era, meiji.eraYear);

const meijiLate = new Temporal.PlainDate(1912, 7, 29, "japanese");
console.log(meijiLate.era, meijiLate.eraYear);

const preMeiji = new Temporal.PlainDate(1868, 9, 7, "japanese");
console.log(preMeiji.era, preMeiji.eraYear);

const bc = new Temporal.PlainDate(-5, 1, 1, "japanese");
console.log(bc.era, bc.eraYear);

const iso = new Temporal.PlainDate(2024, 6, 30);
console.log(Temporal.PlainDate.compare(d, iso));
console.log(d.equals(iso));

const upper = new Temporal.PlainDate(2024, 6, 30).withCalendar("JAPANESE");
console.log(upper.calendarId);

const dt = new Temporal.PlainDateTime(2024, 6, 30, 12, 34, 56, 0, 0, 0, "japanese");
console.log(dt.toString());
console.log(dt.era, dt.eraYear);
