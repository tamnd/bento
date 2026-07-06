// U+2118 (script capital P) is a legal JavaScript identifier character but
// not a Go letter, so a name carrying it escapes to a U-hex spelling: ℘ in a
// function name becomes U2118_ and the rest of the name rides along verbatim.
// The same escape applies to a local, so the call below and the declaration
// agree without any shared table.
function ℘value(n: number): number {
  return n * 2;
}

let ℘x = ℘value(21);
console.log(℘x);
