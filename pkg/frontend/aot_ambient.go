package frontend

import "github.com/tamnd/bento/pkg/goimport"

// This file gives the AOT frontend the Node globals a real program uses. The
// TypeScript standard libraries declare Math, Number, String, and JSON, but not
// process or its streams, so a workload that reads process.env or writes
// process.stdout does not type-check under a stock strict configuration (the
// checker reports "Cannot find name 'process'"). bento does not want a caller to
// install @types/node to compile such a program, so it injects its own ambient
// declarations for the surface the compiler lowers, and no more: declaring only
// what lowers keeps the honest boundary, since a use of an undeclared global
// still errors rather than compiling to nothing.

// ambientPath is the virtual path of the synthetic library file bento adds to
// every AOT load. It is a .d.ts so its declarations are ambient globals, which is
// also what makes isAmbientGlobal treat process as library-provided rather than a
// user binding, the same test that separates the global Math from a local shadow.
const ambientPath = "/__bento_ambient__.d.ts"

// ambientSource declares the Node globals and node: modules the AOT compiler can
// lower. process.env is a string-or-undefined map, which lowers to the optional
// machinery; the streams' write takes a string and returns a boolean, matching
// Node. __dirname and __filename are the CommonJS module-path globals, each a
// string the lowerer fills from the module's own file path, so a program reading
// either resolves the same absolute path Node hands its wrapper. module and
// exports are the CommonJS export globals, typed any so a read of module.exports
// or a write of exports.x lowers through the dynamic member path; the lowerer
// backs them with a package-level module object. require is the CommonJS loader
// global, typed any so typeof require is "function" and a require(specifier) call
// lowers through the dynamic call path; the lowerer backs it with a package-level
// require function value. The node:fs, node:os, and node:path module declarations give the file
// read and write surface a syscall workload uses without a caller installing
// @types/node, each function typed exactly as bento lowers it (readFileSync only
// in its encoding-and-string form, rmSync with the recursive and force options a
// benchmark passes). The surface is deliberately small: it grows one entry at a
// time as the lowerer learns to emit each one, so a declared name is always a
// lowerable one.
const ambientSource = `interface BentoProcessEnv { [key: string]: string | undefined; }
interface BentoWriteStream { write(chunk: string): boolean; }
interface BentoProcess {
	env: BentoProcessEnv;
	argv: string[];
	stdout: BentoWriteStream;
	stderr: BentoWriteStream;
}
declare var process: BentoProcess;
declare var __dirname: string;
declare var __filename: string;
declare var module: any;
declare var exports: any;
declare var require: any;
declare module "node:fs" {
	export function mkdtempSync(prefix: string): string;
	export function writeFileSync(path: string, data: string): void;
	export function readFileSync(path: string, encoding: "utf8"): string;
	export function rmSync(path: string, options?: { recursive?: boolean; force?: boolean }): void;
}
declare module "node:os" {
	export function tmpdir(): string;
}
declare module "node:path" {
	export function join(...parts: string[]): string;
}
`

// ambientText is the full ambient library the overlay serves: the Node globals
// above followed by the bento:go vocabulary as a declare-module block, so a
// program that imports a go: package gets both without either living on disk. The
// vocabulary is owned by the goimport package, which is the single source of truth
// the generated .d.ts files draw their helper names from, so serving it from there
// keeps the declarations a generated file imports and the declarations the checker
// reads from ever drifting apart.
func ambientText() string {
	return ambientSource + goimport.AmbientModule()
}

// ambientOverlay serves the synthetic ambient library on top of a base
// FileSystem, so the checker reads bento's Node declarations without them living
// on disk. It overlays only the one virtual path; every other read falls through
// to the base, so a real project's files are untouched. It wraps the FS the host
// reads through and not the FS the resolver reads through, because the ambient
// file is a compile root rather than an imported module, so the resolver never
// looks for it and the base FS keeps its identity for the osFileSystem fast path.
type ambientOverlay struct{ base FileSystem }

func (a ambientOverlay) ReadFile(path string) (string, bool) {
	if path == ambientPath {
		return ambientText(), true
	}
	return a.base.ReadFile(path)
}

func (a ambientOverlay) FileExists(path string) bool {
	if path == ambientPath {
		return true
	}
	return a.base.FileExists(path)
}

func (a ambientOverlay) DirectoryExists(path string) bool {
	return a.base.DirectoryExists(path)
}
