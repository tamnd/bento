// Temporal.ZonedDateTime.prototype.startOfDay returns the first instant of the local day in the
// zone, wall-clock midnight resolved through the compatible rule, and hoursInDay reads the day's
// length off the two adjacent midnights. An ordinary day is twenty-four hours, the spring-forward
// day loses an hour to twenty-three and its midnight still sits at the standard offset, and the
// fall-back day gains an hour to twenty-five with its midnight at the daylight offset. Every result
// was checked against @js-temporal/polyfill.
const normal = Temporal.ZonedDateTime.from("2024-06-15T12:30:45[America/New_York]");
console.log(normal.startOfDay().toString());
console.log(normal.hoursInDay);

const spring = Temporal.ZonedDateTime.from("2024-03-10T15:00:00[America/New_York]");
console.log(spring.startOfDay().toString());
console.log(spring.hoursInDay);

const fall = Temporal.ZonedDateTime.from("2024-11-03T15:00:00[America/New_York]");
console.log(fall.startOfDay().toString());
console.log(fall.hoursInDay);
