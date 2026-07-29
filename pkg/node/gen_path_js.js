// Generates js/path.js, the node:path the engine serves, from the path module of
// the Node you run this with:
//
//	node pkg/node/gen_path_js.js > pkg/node/js/path.js
//
// path.js used to be written by hand and was an approximation: it had one code
// path for both variants, so a Windows drive was just another path segment,
// relative compared paths case-sensitively, and toNamespacedPath handed its
// argument back on every platform. None of that is what Node does, and the AOT
// path now carries a real port of Node's algorithms in pkg/value/nodepath.go. The
// engine has to give the same answers as the compiled program, so it gets the
// same algorithms, taken from the same place.
//
// The transformation is mechanical: Node's lib/path.js is written against
// primordials, the frozen copies of the built-in methods that keep the standard
// library working when a program overwrites Array.prototype.slice, and this
// rewrites each of those calls back into the method call it stands for. Nothing
// else about the logic is touched, which is the point: a hand-edit is a chance to
// introduce exactly the kind of difference this file exists to remove.
//
// What is dropped: path.matchesGlob, which calls into Node's glob matcher, a
// module of its own that bento does not have yet.

'use strict';

const PRIMORDIALS = {
  ArrayPrototypeIncludes: 'includes',
  ArrayPrototypeJoin: 'join',
  ArrayPrototypePush: 'push',
  ArrayPrototypeSlice: 'slice',
  FunctionPrototypeBind: 'bind',
  StringPrototypeCharCodeAt: 'charCodeAt',
  StringPrototypeIncludes: 'includes',
  StringPrototypeIndexOf: 'indexOf',
  StringPrototypeLastIndexOf: 'lastIndexOf',
  StringPrototypeRepeat: 'repeat',
  StringPrototypeReplace: 'replace',
  StringPrototypeSlice: 'slice',
  StringPrototypeSplit: 'split',
  StringPrototypeToLowerCase: 'toLowerCase',
  StringPrototypeToUpperCase: 'toUpperCase',
};

// splitArgs splits the text between a call's parentheses on its top-level commas.
// It tracks brackets and quotes rather than counting characters, because the
// arguments hold template literals whose ${} parts hold calls of their own.
function splitArgs(text) {
  const out = [];
  let depth = 0;
  let start = 0;
  let quote = null;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quote) {
      if (c === '\\') i++;
      else if (c === quote) quote = null;
      else if (quote === '`' && c === '$' && text[i + 1] === '{') { depth++; i++; }
      continue;
    }
    if (c === "'" || c === '"' || c === '`') { quote = c; continue; }
    if (c === '(' || c === '[' || c === '{') depth++;
    else if (c === ')' || c === ']' || c === '}') depth--;
    else if (c === ',' && depth === 0) { out.push(text.slice(start, i)); start = i + 1; }
  }
  out.push(text.slice(start));
  return out.map((s) => s.trim());
}

// callEnd returns the index just past the parenthesis that closes the one at open.
function callEnd(src, open) {
  let depth = 0;
  let quote = null;
  for (let i = open; i < src.length; i++) {
    const c = src[i];
    if (quote) {
      if (c === '\\') i++;
      else if (c === quote) quote = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') { quote = c; continue; }
    if (c === '(') depth++;
    else if (c === ')') { depth--; if (depth === 0) return i + 1; }
  }
  throw new Error('unbalanced call at ' + open);
}

// deprimordialize rewrites every primordial call into the method call it stands
// for. It works from the rightmost match inwards, so the arguments of the call it
// is rewriting have already been rewritten themselves.
function deprimordialize(src) {
  const names = Object.keys(PRIMORDIALS);
  for (;;) {
    let at = -1;
    let name = null;
    for (const n of names) {
      for (let i = src.length; ;) {
        i = src.lastIndexOf(n + '(', i - 1);
        if (i < 0) break;
        const before = i === 0 ? '' : src[i - 1];
        if (/[A-Za-z0-9_$.]/.test(before)) continue;
        if (i > at) { at = i; name = n; }
        break;
      }
    }
    if (at < 0) break;
    const open = at + name.length;
    const end = callEnd(src, open);
    const args = splitArgs(src.slice(open + 1, end - 1));
    const receiver = args.shift();
    const call = `${receiver}.${PRIMORDIALS[name]}(${args.join(', ')})`;
    src = src.slice(0, at) + call + src.slice(end);
  }
  return src;
}

// dropMethod removes a whole method from an object literal, brace matching from
// its opening line so the body goes with it.
function dropMethod(src, signature) {
  const at = src.indexOf(signature);
  if (at < 0) throw new Error('no method ' + signature);
  let i = src.indexOf('{', at);
  let depth = 0;
  for (; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}') { depth--; if (depth === 0) break; }
  }
  let end = i + 1;
  while (end < src.length && (src[end] === ',' || src[end] === '\n')) end++;
  return src.slice(0, at) + src.slice(end);
}

const source = process.binding('natives').path;

// Everything above the first function is the license, the primordials and the
// internal requires, none of which survive the trip. Everything below the last
// assignment is the CommonJS export, which the wrapper does instead.
let body = source.slice(source.indexOf('function isPathSeparator('));
body = body.slice(0, body.indexOf('module.exports ='));

body = dropMethod(body, '  matchesGlob(path, pattern) {');
body = dropMethod(body, '  matchesGlob(path, pattern) {');
body = deprimordialize(body);
body = body.replace(/\n{3,}/g, '\n\n').trimEnd();

const header = `// path implements node:path, both variants, as Node implements them.
//
// This file is generated by gen_path_js.js next to it, which rewrites Node's own
// lib/path.js into plain JavaScript. Edit that script rather than this file, and
// regenerate:
//
//	node pkg/node/gen_path_js.js > pkg/node/js/path.js
//
// The engine and the ahead-of-time path have to answer alike, and the compiled
// program calls a port of the same Node source (pkg/value/nodepath.go), so both
// sides of bento take their path answers from one place. pkg/node/path_test.go
// holds this file to a table of real Node output, the same table the Go port is
// held to.
//
// Generated from Node ${process.version}. Do not edit.

__bento_defineModule("path", function (module, exports, require) {
  "use strict";

  // The character codes the algorithms compare against, which Node keeps in
  // internal/constants.
  const CHAR_UPPERCASE_A = 65;
  const CHAR_LOWERCASE_A = 97;
  const CHAR_UPPERCASE_Z = 90;
  const CHAR_LOWERCASE_Z = 122;
  const CHAR_DOT = 46;
  const CHAR_FORWARD_SLASH = 47;
  const CHAR_BACKWARD_SLASH = 92;
  const CHAR_COLON = 58;
  const CHAR_QUESTION_MARK = 63;

  // isWindows picks which variant the module exports, the way Node's does. It
  // reads the platform off process rather than off a build flag, so a program
  // that has replaced process.platform sees the path module that goes with it.
  const isWindows = typeof process !== "undefined" && process.platform === "win32";

  // The two validators Node's path calls, kept to the same messages: a program
  // that catches one of these reads its message.
  function validateString(value, name) {
    if (typeof value !== "string") {
      throw new TypeError(
        'The "' + name + '" argument must be of type string. Received ' +
          (value === null ? "null" : typeof value)
      );
    }
  }

  function validateObject(value, name) {
    if (value === null || typeof value !== "object" || Array.isArray(value)) {
      throw new TypeError(
        'The "' + name + '" argument must be of type object. Received ' +
          (value === null ? "null" : typeof value)
      );
    }
  }

`;

const footer = `
  module.exports = isWindows ? win32 : posix;
});
`;

const indented = body.split('\n').map((l) => (l.length ? '  ' + l : l)).join('\n');
process.stdout.write(header + indented + '\n' + footer);
