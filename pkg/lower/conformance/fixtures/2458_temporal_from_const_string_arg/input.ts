// A const string passed straight to a Temporal from-string lowering: the from call
// constant-folds the const to its literal value and emits the quoted string, so the
// identifier is never lowered. Without recording that dropped read the const would be
// declared and not used and the emitted Go would not build. The same fold applies to a
// time-zone string handed to withTimeZone, so both arms exercise the orphaned binding.
const iso = "1976-11-18";
const d = Temporal.PlainDate.from(iso);
console.log(d.year);
console.log(d.month);
console.log(d.day);
console.log(d.calendarId);

const zone = "UTC";
const zdt = Temporal.ZonedDateTime.from("2020-01-01T00:00:00+00:00[UTC]").withTimeZone(zone);
console.log(zdt.timeZoneId);
