// The Final_Sigma context decides whether a capital sigma lowercases to the
// word-final form ς or the ordinary σ. The rule scans past case-ignorable
// characters on both sides, and U+0345 COMBINING GREEK YPOGEGRAMMENI is both cased
// and case-ignorable. In "ͅΣ" the ypogegrammeni is consumed as case-ignorable, so
// no cased letter sits before the sigma and it lowercases to σ, not ς. x/text
// treats the leading ypogegrammeni as the cased letter and gives the final ς, so
// the runtime runs the sigma decision itself. The remaining lines pin the rest of
// the context grid: a cased letter before an ignorable is final, and a cased letter
// after an ignorable is not.
console.log("ͅΣ".toLowerCase());
console.log("ᾼΣ".toLowerCase());
console.log("AΣͅΑ".toLowerCase());
console.log("AΣ".toLowerCase());
console.log("ΟΔΟΣ".toLowerCase());
