// isWellFormed reports whether a string holds any lone surrogate, and toWellFormed
// returns a copy with each lone surrogate replaced by the U+FFFD replacement
// character. A string of valid code points, including an astral character built
// from a surrogate pair, is well-formed and passes through toWellFormed unchanged;
// a string carrying a lone surrogate is not well-formed and toWellFormed repairs
// it, so the result is well-formed and its lone unit becomes U+FFFD (charcode
// 65533).
function well(s: string): boolean {
  return s.isWellFormed();
}

function run(): void {
  const plain = "hello";
  const astral = String.fromCodePoint(0x1f600);
  const lone = String.fromCharCode(0x61, 0xd83d, 0x62);

  console.log(well(plain));
  console.log(well(astral));
  console.log(well(lone));

  console.log(plain.toWellFormed() === plain);
  console.log(lone.toWellFormed().isWellFormed());
  console.log(lone.toWellFormed().length);
  console.log(lone.toWellFormed().charCodeAt(1));
}

run();
