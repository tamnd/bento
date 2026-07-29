// Exercise the node:path surface a program actually reaches for. Every call here
// answers the same way whatever directory the process is in, which is what lets
// the engine and the compiled program be compared: the two run from different
// places, and a call that read the working directory would differ for that reason
// rather than for a real one.
import {
  join,
  normalize,
  dirname,
  basename,
  extname,
  isAbsolute,
  relative,
} from "node:path";

console.log(join("a", "b", "c.txt"));
console.log(join("a/b/", "../c"));
console.log(join());
console.log(normalize("a/./b/../c/"));
console.log(normalize("/a//b/../c"));
console.log(dirname("/a/b/c.txt"));
console.log(dirname("a"));
console.log(basename("/a/b/c.txt"));
console.log(basename("/a/b/c.txt", ".txt"));
console.log(basename(""));
console.log(extname("/a/b/c.txt"));
console.log(extname("/a/.bashrc"));
console.log(extname("file."));
console.log(isAbsolute("/a"));
console.log(isAbsolute("a"));
console.log(relative("/a/b", "/a/b/c/d"));
console.log(relative("/a/b/c", "/a/x"));
console.log(relative("/a/b", "/a/b"));
