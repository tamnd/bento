// normalize rewrites a string into one of the four Unicode normalization forms so
// that text that reads the same compares the same. The A-with-ring letter has a
// precomposed form of one code point and a decomposed form of an A followed by a
// combining ring, and NFC composes the decomposed form into the precomposed one
// while NFD does the reverse. Omitting the form defaults to NFC. The compatibility
// form NFKC additionally folds the fi ligature into the two letters f and i, which
// the canonical NFC leaves alone.
function run(): void {
  const decomposed = "Å";
  const precomposed = "Å";

  console.log(decomposed.normalize("NFC") === precomposed);
  console.log(decomposed.length);
  console.log(decomposed.normalize("NFC").length);
  console.log(precomposed.normalize("NFD") === decomposed);
  console.log(decomposed.normalize() === precomposed);

  const lig = "ﬁ";
  console.log(lig.normalize("NFKC"));
  console.log(lig.normalize("NFC") === lig);
}

run();
