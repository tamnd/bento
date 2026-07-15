// A Temporal.ZonedDateTime is a TimeZoneLike: when it is handed to a Temporal.Now ISO function the
// spec reads the value's own time zone and reports the current instant against it. The clock is
// pinned (BENTO_NOW_NS) to 2023-11-14T22:13:20.123456789Z so the wall-clock projection is exact.

const ny = new Temporal.ZonedDateTime(0n, "America/New_York");
console.log(Temporal.Now.zonedDateTimeISO(ny).toString());
console.log(Temporal.Now.plainDateTimeISO(ny).toString());
console.log(Temporal.Now.plainDateISO(ny).toString());
console.log(Temporal.Now.plainTimeISO(ny).toString());

const tokyo = new Temporal.ZonedDateTime(0n, "Asia/Tokyo");
console.log(Temporal.Now.zonedDateTimeISO(tokyo).toString());
console.log(Temporal.Now.plainDateISO(tokyo).toString());
console.log(Temporal.Now.plainTimeISO(tokyo).toString());
