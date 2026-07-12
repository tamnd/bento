// Temporal.Duration.toString renders the ISO 8601 duration form: an optional leading minus,
// then P and the non-zero date components, then T and the non-zero time components. The
// seconds field folds seconds, milliseconds, microseconds, and nanoseconds into one decimal
// with the fraction trimmed of trailing zeros. An all-zero duration renders as "PT0S", and
// toJSON produces the same string. The values match @js-temporal/polyfill.
console.log(new Temporal.Duration(1, 2, 3, 4, 5, 6, 7, 8, 9, 10).toString());
console.log(new Temporal.Duration(5).toString());
console.log(new Temporal.Duration(0, 0, 3).toString());
console.log(new Temporal.Duration(0, 0, 0, 4, 0, 0, 0, 500).toString());
console.log(new Temporal.Duration(0, 0, 0, 0, 0, 0, 0, 0, 0, 500).toString());
console.log(new Temporal.Duration(0, 0, 0, 0, 0, 90).toString());
console.log(new Temporal.Duration(0, 0, 0, 0, 0, 0, -1, -500).toString());
console.log(new Temporal.Duration().toString());
console.log(new Temporal.Duration(1, 2, 3).toJSON());
