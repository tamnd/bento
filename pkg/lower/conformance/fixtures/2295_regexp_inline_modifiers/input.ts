// An inline flag modifier changes a flag for the group it wraps: (?i:...) folds case
// inside its scope, (?-i:...) restores case sensitivity inside a case-insensitive
// regexp, and (?s:.) makes the dot match a line terminator only within the group. RE2
// shares the i and s spelling and their meaning, so each of these lowers unchanged.
console.log(/a(?i:b)c/.test("aBc")); // true, (?i:b) folds the b for its scope
console.log(/a(?i:b)c/.test("ABc")); // false, a outside the scope stays case-sensitive
console.log(/a(?-i:b)c/i.test("AbC")); // true, global i folds a and c, b stays exact
console.log(/a(?-i:b)c/i.test("ABC")); // false, (?-i:b) restores case sensitivity for b
console.log(/x(?s:.)y/.test("x\ny")); // true, (?s:.) matches the newline in its scope
console.log(/x.y/.test("x\ny")); // false, the plain dot excludes a line terminator
console.log(/a(?i:b(?-i:c))d/.test("aBcd")); // true, nested modifiers layer their scopes
console.log(/a(?i:b(?-i:c))d/.test("aBCd")); // false, the inner (?-i:c) needs an exact c
