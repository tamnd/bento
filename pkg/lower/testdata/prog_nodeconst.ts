// Reads the data exports of node:path and node:os in the two forms a program
// writes them in: a member of a module-object binding, and a bare name a named
// import bound. Every line's answer is a fact about the platform the binary was
// built for, so the expectations live in the test, per platform, taken from Node.
import * as path from "node:path";
import { sep } from "node:path";
import { EOL, devNull } from "node:os";

console.log(path.sep);
console.log(path.delimiter);
console.log(sep === path.sep);
console.log(devNull);
console.log(EOL.length);
console.log(path.join("a", "b") === "a" + sep + "b");
