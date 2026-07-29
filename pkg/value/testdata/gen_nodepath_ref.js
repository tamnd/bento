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

// The objects format is driven with. Most of them are there for one precedence
// rule each: base beats name and ext, dir beats root, and a dir that is the root
// joins without a separator because the root already ends in one.
const FORMATS = [
  {},
  { root: '/' },
  { root: 'C:\\' },
  { dir: '/a' },
  { base: 'f.txt' },
  { name: 'f' },
  { ext: '.txt' },
  { ext: 'txt' },
  { name: 'f', ext: '.txt' },
  { name: 'f', ext: 'txt' },
  { dir: '/a', base: 'f.txt' },
  { dir: '/a/b', name: 'f', ext: '.txt' },
  { root: '/', base: 'f.txt' },
  { root: '/', name: 'f', ext: '.txt' },
  { dir: '/', base: 'f.txt' },
  { dir: '/a', root: '/', base: 'f.txt' },
  { dir: '/a', base: 'b.txt', name: 'f', ext: '.js' },
  { root: 'C:\\', base: 'f.txt' },
  { dir: 'C:\\a', base: 'f.txt' },
  { dir: 'C:\\', base: 'f.txt' },
  { dir: 'C:\\', root: 'C:\\', base: 'f.txt' },
  { dir: '', base: '' },
  { name: '', ext: '' },
  { dir: 'a', base: 'b' },
  { dir: 'a', name: 'b' },
];

function variant(impl, cwd) {
  const realCwd = process.cwd;
  process.cwd = () => cwd;
  for (const key of Object.keys(process.env)) {
    if (key.startsWith('=')) delete process.env[key];
  }
  try {
    const out = { cwd, unary: {}, basenameExt: {}, join: {}, resolve: {}, relative: {}, parse: {}, format: {} };
    for (const f of FORMATS) {
      out.format[JSON.stringify(f)] = impl.format(f);
    }
    for (const p of PATHS) {
      out.parse[p] = impl.parse(p);
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
