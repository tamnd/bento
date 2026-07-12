// Temporal.PlainYearMonth.prototype.add shifts a year-month by a duration, which needs the
// duration balancing and the calendar model this slice does not carry, so it hands back with a
// named reason rather than emit wrong Go. The same ceiling covers subtract, until, since, with,
// from over a string, and toPlainDate.
function addMonths(): void {
  const ym = new Temporal.PlainYearMonth(2020, 3);
  ym.add({ months: 1 });
}
