package frontend

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

// ambientSource declares the Node globals the AOT compiler can lower. process.env
// is a string-or-undefined map, which lowers to the optional machinery; the
// streams' write takes a string and returns a boolean, matching Node. The surface
// is deliberately small: it grows one entry at a time as the lowerer learns to
// emit each one, so a declared global is always a lowerable one.
const ambientSource = `interface BentoProcessEnv { [key: string]: string | undefined; }
interface BentoWriteStream { write(chunk: string): boolean; }
interface BentoProcess {
	env: BentoProcessEnv;
	argv: string[];
	stdout: BentoWriteStream;
	stderr: BentoWriteStream;
}
declare var process: BentoProcess;
`

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
		return ambientSource, true
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
