// The WebCrypto global is four names on the global object: the one crypto instance
// a host installs and the three classes behind it, which is what test/common
// collects into its known-globals set.
const knownGlobals = new Set<unknown>();
knownGlobals.add(globalThis.crypto);
knownGlobals.add(globalThis.Crypto);
knownGlobals.add(globalThis.CryptoKey);
knownGlobals.add(globalThis.SubtleCrypto);
console.log(knownGlobals.size, knownGlobals.has(crypto), knownGlobals.has(SubtleCrypto));

// The bare name, the member off the global object and Node's other spelling of the
// global scope all reach the one object.
console.log(crypto === globalThis.crypto, crypto === global.crypto);
console.log(typeof crypto, typeof crypto.subtle);

// The three class names report themselves the way any class does.
console.log(typeof Crypto, typeof CryptoKey, typeof SubtleCrypto);

// The two members that carry state report themselves too, and a program feature-
// tests them this way before it calls either.
console.log(typeof crypto.randomUUID, typeof crypto.getRandomValues, typeof crypto.subtle.digest);

// randomUUID is a real version 4 UUID: thirty-six characters in five groups with
// the version nibble at index 14, and a fresh one on every call.
const id = crypto.randomUUID();
console.log(id.length, id.charAt(14), id.split("-").length);
console.log(crypto.randomUUID() === crypto.randomUUID());

// getRandomValues fills the array it was given and hands that same array back, so a
// caller that ignores the return still has its bytes.
const bytes = new Uint8Array(16);
console.log(crypto.getRandomValues(bytes) === bytes, bytes.length);

// A float view is not an integer-type typed array, and the refusal names itself.
try {
  crypto.getRandomValues(new Float64Array(4));
} catch (e) {
  console.log(String(e));
}

// A binding a function declares is its own, whatever the global scope calls the
// same name.
function shadowed(): number {
  const crypto = 3;
  return crypto;
}
console.log(shadowed());
