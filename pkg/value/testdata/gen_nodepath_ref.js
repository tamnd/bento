// Generates nodepath_node24.json, the table pkg/value/nodepath_test.go holds the
// path port to. Run it with the Node whose answers you want to pin:
//
//	node pkg/value/testdata/gen_nodepath_ref.js > pkg/value/testdata/nodepath_node24.json
//
// Both variants are generated on whatever machine you run this on, because Node
// exposes path.posix and path.win32 everywhere and both are the same code that
// runs on the platform they are named for.
//
// process.cwd is stubbed for each variant, since resolve and relative read it and
// an answer that depended on where this was run would not be checkable anywhere
// else. The drive-specific working directories Windows keeps, which Node reads
// out of the environment as "=C:", are cleared for the same reason.

'use strict';

const path = require('path');

const PATHS = [
  '', '.', '..', '/', '//', '///',
  'a', 'a/b', 'a/b/', '/a', '/a/', '/a/b',
  'a//b', 'a/./b', 'a/../b', '../a', 'a/..', 'a/../..', '/..', '/../a', './a',
  'a/b/c.txt', 'c.txt', '.bashrc', 'a/.bashrc', 'file.', 'file..', 'a.b.c',
  '/a/b/c.txt', 'a/b/../c', 'foo/bar//baz', ' ', 'a b/c d',
  'C:', 'C:\\', 'C:\\a', 'C:\\a\\b', 'C:/a/b',
  '\\\\server\\share', '\\\\server\\share\\a',
  '\\a', '\\a\\b', 'a\\b', 'a\\b\\', '\\', '\\\\',
  'C:a', 'c:\\A\\..\\b', '//server/share/x', 'a/b\\c',
];

const SUFFIXES = ['', '.txt', '.c.txt', 'b', '/', '.bashrc'];
const SECONDS = ['b', '..', '/b', 'C:\\a', '', 'a/b'];

function variant(impl, cwd) {
  const realCwd = process.cwd;
  process.cwd = () => cwd;
  for (const key of Object.keys(process.env)) {
    if (key.startsWith('=')) delete process.env[key];
  }
  try {
    const out = { cwd, unary: {}, basenameExt: {}, join: {}, resolve: {}, relative: {} };
    for (const p of PATHS) {
      out.unary[p] = {
        normalize: impl.normalize(p),
        dirname: impl.dirname(p),
        basename: impl.basename(p),
        extname: impl.extname(p),
        isAbsolute: impl.isAbsolute(p),
        toNamespacedPath: impl.toNamespacedPath(p),
      };
      for (const s of SUFFIXES) {
        out.basenameExt[JSON.stringify([p, s])] = impl.basename(p, s);
      }
      out.resolve[JSON.stringify([p])] = impl.resolve(p);
      for (const q of SECONDS) {
        const key = JSON.stringify([p, q]);
        out.join[key] = impl.join(p, q);
        out.relative[key] = impl.relative(p, q);
        out.resolve[key] = impl.resolve(p, q);
      }
    }
    out.resolve[JSON.stringify([])] = impl.resolve();
    out.sep = impl.sep;
    out.delimiter = impl.delimiter;
    return out;
  } finally {
    process.cwd = realCwd;
  }
}

const ref = {
  nodeVersion: process.version,
  posix: variant(path.posix, '/cwd'),
  win32: variant(path.win32, 'C:\\cwd'),
};

process.stdout.write(JSON.stringify(ref, null, 1) + '\n');
