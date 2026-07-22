// Package build is bento's ahead-of-time path: it type-checks an entry module
// with the real frontend, lowers it to a Go program, and compiles that program
// to a native binary with the Go toolchain. It is the code behind `bento build`,
// and the number bento's AOT benchmark column measures.
//
// The generated program imports bento's own value package for its runtime
// primitives (the UTF-16 string model, the exact Number::toString, the console
// helpers), so it is compiled inside the bento module tree, where that import
// and its transitive dependencies already resolve from the module's go.mod and
// go.sum with no network. The module root is discovered from the running binary
// or an explicit override; a build that cannot find it fails with a clear
// message rather than emitting a program that will not link.
package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/goimport"
	"github.com/tamnd/bento/pkg/lower"
)

// Options controls a single build. Entry is the path to the TypeScript or
// JavaScript entry module; Output is where the native binary is written. A blank
// Output defaults to the entry's base name without its extension, in the current
// directory, matching how a Go or C compiler names its output. AllowCgo
// acknowledges that a go: import pulling in cgo forfeits the zero-cgo
// cross-compile, letting such a build proceed instead of stopping with the section
// 9.5 diagnostic; without it, a build sets CGO_ENABLED=0 and a cgo import fails
// loudly at detection time rather than silently linking a cgo binary.
type Options struct {
	Entry    string
	Output   string
	AllowCgo bool
}

// EmitOptions carries the case-level compiler settings bento honors when it
// checks and gates a program, the parts of a tsconfig that change whether the
// checker's rejection stands. A zero value is bento's default project, which
// the CLI uses until tsconfig discovery lands; the AOT conformance harness
// supplies them per case so bento checks a case under the same options
// TypeScript did.
type EmitOptions struct {
	// NoImplicitAny is the project's effective noImplicitAny setting, set
	// directly or implied by strict. bento always checks strictly to get the
	// most precise types it can, so the checker reports an untyped form whether
	// or not the project asked for it; this flag records whether the project
	// actually asked. When it did, the report is a rejection TypeScript stands
	// behind, so bento hands the unit back rather than lowering the untyped form
	// to a dynamic value. When it did not, the report is bento's own added
	// strictness and the form lowers as before.
	NoImplicitAny bool
	// Target names the emit target the checker resolves against, for example
	// "es2015". An empty string keeps bento's esnext default. It matters for the
	// diagnostics that depend on downleveling, such as the helper a pre-es2017
	// async function needs under ImportHelpers.
	Target string
	// ImportHelpers reproduces a project built with the tslib helper library, so
	// the checker reports the helper a downlevel construct would need but the
	// program does not import.
	ImportHelpers bool
	// AllowUnreachableCode, when non-nil, sets allowUnreachableCode. A nil pointer
	// keeps the checker's default of reporting unreachable code; setting it false
	// is what makes an unreachable statement a build-stopping error rather than
	// dead code the lowerer would emit.
	AllowUnreachableCode *bool
}

// Build type-checks the entry module, lowers it to a Go program, and compiles
// that program to a native binary at Options.Output. A type error in the entry,
// a construct the lowerer does not yet cover, or a Go toolchain failure is
// returned as an error, so the caller (the CLI, the benchmark harness) reports a
// real failure rather than a binary that does not match the source.
func Build(opts Options) error {
	entry, err := filepath.Abs(opts.Entry)
	if err != nil {
		return fmt.Errorf("bento build: %s: %w", opts.Entry, err)
	}
	if _, err := os.Stat(entry); err != nil {
		return fmt.Errorf("bento build: %s: %w", opts.Entry, err)
	}

	output := opts.Output
	if output == "" {
		base := filepath.Base(entry)
		output = strings.TrimSuffix(base, filepath.Ext(base))
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("bento build: output %s: %w", opts.Output, err)
	}

	source, goPaths, err := compileProgram(entry, EmitOptions{})
	if err != nil {
		return err
	}
	needsCgo, err := gateCgo(goPaths, opts.AllowCgo)
	if err != nil {
		return err
	}
	return link(source, output, needsCgo)
}

// EmitGo type-checks the entry module, lowers it to a Go program, and returns
// that program's source prefixed with a header that names the bento build which
// produced it. It runs the front half of Build and stops before the toolchain,
// so a caller can write the Go to a golden file or read it. stamp is a short
// build identifier, a version and commit, recorded in the header so a
// checked-in golden always names the compiler that generated it. The header is
// the standard "Code generated ... DO NOT EDIT." marker, so Go tooling treats a
// golden as generated and never reformats or vets it as hand-written source.
func EmitGo(entry, stamp string) (string, error) {
	return EmitGoWithOptions(entry, stamp, EmitOptions{})
}

// EmitGoWithOptions is EmitGo under an explicit project configuration. It lets a
// caller that knows the case's compiler options, the AOT conformance harness,
// check and gate the case the way TypeScript would rather than under bento's
// fixed defaults; EmitGo is the same call with the default project.
func EmitGoWithOptions(entry, stamp string, opts EmitOptions) (string, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
		return "", fmt.Errorf("bento build: %s: %w", entry, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("bento build: %s: %w", entry, err)
	}
	source, _, err := compileProgram(abs, opts)
	if err != nil {
		return "", err
	}
	header := fmt.Sprintf("// Code generated by bento %s from %s. DO NOT EDIT.\n\n", stamp, filepath.Base(abs))
	return header + source, nil
}

// Compile type-checks the entry module and returns the Go source of the program
// it lowers to. It is the front half of Build, split out so a caller can inspect
// or golden the generated Go without running the toolchain.
func Compile(entry string) (string, error) {
	source, _, err := compileProgram(entry, EmitOptions{})
	return source, err
}

// compileProgram is the shared front half of the build: it type-checks the entry,
// lowers it to a Go program, and returns the source together with the Go import
// paths the program reaches through a go: call. Build consults those paths to
// detect cgo before it runs the toolchain (section 9.5); Compile and EmitGo, which
// stop before the toolchain, ignore them.
// overridesFor maps the project settings a caller supplies onto the frontend's
// config overrides, so the checker resolves the case under the same options
// TypeScript did rather than bento's fixed defaults. Only the settings a case
// actually changes are folded on; bento keeps strict checking and its esnext
// default for everything a case leaves unset. NoImplicitAny is not folded here:
// bento already checks strictly, so the checker reports the untyped form
// regardless, and whether that report gates is decided in firstError.
func overridesFor(opts EmitOptions) frontend.ConfigOverrides {
	ov := frontend.ConfigOverrides{
		Target:               opts.Target,
		AllowUnreachableCode: opts.AllowUnreachableCode,
	}
	if opts.ImportHelpers {
		t := true
		ov.ImportHelpers = &t
	}
	return ov
}

func compileProgram(entry string, opts EmitOptions) (string, []string, error) {
	if isJavaScript(entry) {
		return "", nil, fmt.Errorf("bento build: %s: JavaScript entries are a later slice; the AOT path compiles TypeScript (.ts) today", entry)
	}
	// Resolve the entry's symlinks before loading so a build from a symlinked
	// directory finds its sibling modules. The checker resolves a relative import
	// against the entry's canonical directory, so an entry named through a symlink
	// (macOS /tmp and /var link into /private) would look for the sibling beside the
	// canonical path and miss the one beside the link. Canonicalizing here keeps the
	// entry and its resolved siblings on one spelling for the whole build.
	entry = canonicalPath(entry)
	prog, err := frontend.Load(frontend.LoadOptions{Roots: []string{entry}, Overrides: overridesFor(opts)})
	if err != nil {
		return "", nil, fmt.Errorf("bento build: %s: %w", entry, err)
	}
	if diag := firstError(prog, opts); diag != "" {
		return "", nil, fmt.Errorf("bento build: %s: %s", entry, diag)
	}

	// A go: import brings its generated .d.ts into the program as a source file, so
	// the declaration files are the interop surface the checker read, not modules to
	// lower. The entry is the root the caller named; a module-goal entry also imports
	// sibling modules the loader resolved, which lower alongside it as one unit.
	// Selecting the roots this way is what lets the AOT build compile a program that
	// imports Go or a sibling module, not just a self-contained one.
	entryFile, deps, err := entryAndDeps(prog, entry)
	if err != nil {
		return "", nil, fmt.Errorf("bento build: %s: %w", entry, err)
	}

	r := lower.NewRenderer(prog)
	r.SetGoSignatures(goSignatureResolver())
	r.SetGoConstants(goConstantResolver())
	r.SetGoErrorVars(goErrorVarResolver())
	p, err := r.RenderProgramModules(entryFile, deps)
	if err != nil {
		return "", nil, fmt.Errorf("bento build: %s: %w", entry, err)
	}
	return p.Source, r.GoImportPaths(), nil
}

// goSignatureResolver builds the resolver the lowerer marshals go: number
// crossings against. It loads each Go package's signatures once and memoizes them,
// including a failed load as an empty set, so a program importing several functions
// from one package pays the go/packages load a single time and a package that will
// not load degrades to the string and boolean crossings rather than failing the
// whole build. The build is single-threaded through here, so the memo needs no
// lock.
func goSignatureResolver() func(importPath, name string) (goimport.FuncSig, bool) {
	memo := map[string]map[string]goimport.FuncSig{}
	return func(importPath, name string) (goimport.FuncSig, bool) {
		sigs, loaded := memo[importPath]
		if !loaded {
			var err error
			sigs, err = goimport.Signatures(importPath)
			if err != nil {
				sigs = map[string]goimport.FuncSig{}
			}
			memo[importPath] = sigs
		}
		sig, ok := sigs[name]
		return sig, ok
	}
}

// goConstantResolver builds the resolver the lowerer marshals a go: constant
// reference against, the companion to goSignatureResolver for a binding used as a
// value. It memoizes each package's constants the same way, sharing the one
// go/packages load and degrading a failed load to an empty set, so a reference to a
// constant from a package that will not load simply hands back.
func goConstantResolver() func(importPath, name string) (goimport.ConstInfo, bool) {
	memo := map[string]map[string]goimport.ConstInfo{}
	return func(importPath, name string) (goimport.ConstInfo, bool) {
		consts, loaded := memo[importPath]
		if !loaded {
			var err error
			consts, err = goimport.Constants(importPath)
			if err != nil {
				consts = map[string]goimport.ConstInfo{}
			}
			memo[importPath] = consts
		}
		info, ok := consts[name]
		return info, ok
	}
}

// goErrorVarResolver builds the resolver the lowerer checks a go: sentinel against
// when a caught error's is() names one, the companion to goConstantResolver for an
// error variable rather than a constant. It memoizes each package's error variables
// the same way, sharing the one go/packages load and degrading a failed load to an
// empty set, so err.is against a sentinel from a package that will not load simply
// hands back.
func goErrorVarResolver() func(importPath, name string) bool {
	memo := map[string]map[string]bool{}
	return func(importPath, name string) bool {
		vars, loaded := memo[importPath]
		if !loaded {
			var err error
			vars, err = goimport.ErrorVars(importPath)
			if err != nil {
				vars = map[string]bool{}
			}
			memo[importPath] = vars
		}
		return vars[name]
	}
}

// entryAndDeps splits the program's lowerable source files into the entry, the
// root the caller named, and the sibling modules that lower alongside it. A go:
// import contributes its generated .d.ts to the program, which supplies the
// interop types to the checker but is not a module to lower, so declaration files
// are skipped. The entry is matched by path against the caller's root, resolving
// the symlinks a temp directory carries so a staged entry under /var matches its
// /private/var form; every other lowerable file is a dependency the entry
// imports, which RenderProgramModules composes into one unit. A program with no
// lowerable module hands back, and a set of lowerable files with no match for the
// root hands back rather than lower an arbitrary one as the entry.
func entryAndDeps(prog *frontend.Program, root string) (frontend.Node, []frontend.Node, error) {
	rootKey := canonicalPath(root)
	var entry frontend.Node
	var deps []frontend.Node
	found := false
	for _, sf := range prog.SourceFiles() {
		if sf.File().Kind == frontend.FileDTS {
			continue
		}
		if !found && canonicalPath(sf.File().Path) == rootKey {
			entry, found = sf, true
			continue
		}
		deps = append(deps, sf)
	}
	if !found {
		// A single lowerable file that did not path-match the root is still the entry:
		// the root names it even when a symlink or a relative form kept the strings
		// apart. With more than one file and no match, the entry is ambiguous, so the
		// unit hands back rather than pick one.
		if len(deps) == 1 {
			return deps[0], nil, nil
		}
		if len(deps) == 0 {
			return nil, nil, fmt.Errorf("no lowerable entry module")
		}
		return nil, nil, fmt.Errorf("cannot identify the entry module among the composed files")
	}
	return entry, deps, nil
}

// canonicalPath resolves a path's symlinks so two spellings of one file compare
// equal, falling back to the path as written when it cannot be resolved (a
// virtual file a test feeds through an in-memory FS has no on-disk form). It is
// how the entry, named by the caller, matches the source file the loader recorded
// even when a temp directory sits behind a symlink.
func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// isJavaScript reports whether the entry is a JavaScript module by extension, so
// the frontend is told to admit and check it. A TypeScript entry leaves the
// options untouched and is checked strictly as usual.
func isJavaScript(entry string) bool {
	switch filepath.Ext(entry) {
	case ".js", ".mjs", ".cjs", ".jsx":
		return true
	default:
		return false
	}
}

// firstError returns the message of the first type error in the program, or the
// empty string when it type-checks cleanly. A build stops on the first error so
// its message is the one the user sees, the same contract the CLI type-checker
// keeps. An implicit-any diagnostic is skipped rather than treated as fatal when
// the project did not ask for noImplicitAny: bento checks strictly to get the
// most precise types it can, but noImplicitAny only reports that a form was left
// untyped, it does not change the resolved type, which is already `any`. The
// lowerer runs `any` through its dynamic value path (the same path an explicit
// `x: any` takes), so an untyped parameter, variable, binding element, or member
// lowers to a dynamic slot instead of being refused. Skipping the diagnostic is
// equivalent to a noImplicitAny:false project for these forms, but scoped to
// bento's AOT gate and kept per-family so only the untyped-form codes are
// tolerated; every other error still gates. When the project did ask for
// noImplicitAny (opts.NoImplicitAny, set directly or through strict), the report
// is a rejection TypeScript stands behind rather than bento's added strictness,
// so it gates like any other error and the unit hands back.
func firstError(prog *frontend.Program, opts EmitOptions) string {
	for _, d := range prog.Diagnostics() {
		if d.Category != frontend.CategoryError {
			continue
		}
		// A property miss on a private name (.#x) is not the tolerable absent-property
		// read that folds to undefined: a private name is valid only inside the class
		// body that declares it, so an access outside any class, or an in-class access
		// of a name the class never declared, is a hard error the build must surface
		// rather than route to the dynamic member path. The checker spells both as a
		// 2339, or its 2551 suggestion variant, over a #-prefixed property, so those
		// gate here ahead of the tolerated-member skip below.
		if isPrivateNameMiss(d) {
			return d.Message
		}
		if !opts.NoImplicitAny && toleratedImplicitAny[d.Code] {
			continue
		}
		if toleratedDynamicMember[d.Code] {
			continue
		}
		if toleratedUnknown[d.Code] {
			continue
		}
		if toleratedLooseForm[d.Code] {
			continue
		}
		if toleratedArity[d.Code] {
			continue
		}
		if toleratedArithOperand[d.Code] {
			continue
		}
		if toleratedDeleteForm[d.Code] {
			continue
		}
		if toleratedComparison[d.Code] {
			continue
		}
		if toleratedComputedKey[d.Code] {
			continue
		}
		if toleratedAssignability[d.Code] {
			// A not-assignable report whose elaboration bottoms out in a signature
			// arity mismatch is not the value coercion the assignability toleration
			// admits: it flags that the source's call or construct signature takes
			// more arguments than the target signature supplies, a `const c: { new():
			// Foo } = Foo` where Foo's constructor needs an argument the target
			// signature never passes. There is no widened literal or cross-primitive
			// value to land here, only two signatures of incompatible arity, so the
			// bridges the toleration leans on have nothing to reconcile and the honest
			// outcome is a handback, not a lowering. The checker nests the arity report
			// (2849) under the 2322/2345, so its presence in the message chain gates
			// the case here ahead of the tolerated skip.
			if hasArityMismatch(d) {
				return d.Message
			}
			continue
		}
		if toleratedOverload[d.Code] {
			continue
		}
		if toleratedIterable[d.Code] {
			continue
		}
		if toleratedImplicitThis[d.Code] {
			continue
		}
		if toleratedConstructAny[d.Code] {
			continue
		}
		if toleratedPossiblyUndefined[d.Code] {
			continue
		}
		return d.Message
	}
	return ""
}

// isPrivateNameMiss reports whether a diagnostic is a property-does-not-exist error
// over a private name. The 2339 and 2551 codes cover both the outside-class access
// and an undeclared in-class access, and both spell the message "Property '#name'
// does not exist ...", so the #-prefixed quoted property is the tell. A normal
// missing property ("Property 'b' ...") is not private and stays tolerated.
func isPrivateNameMiss(d frontend.Diagnostic) bool {
	return (d.Code == 2339 || d.Code == 2551) && strings.HasPrefix(d.Message, "Property '#")
}

// hasArityMismatch reports whether a diagnostic's elaboration chain carries a
// signature arity mismatch, the 2849 "Target signature provides too few
// arguments" report the checker nests under an assignability error when a source
// call or construct signature is assigned to a target signature that supplies
// fewer arguments. The report lives in the diagnostic's Related chain rather than
// its top message, and the chain can nest, so the search walks it in full. A
// caller uses this to separate a signature-shape mismatch, which has no run-time
// value coercion to reproduce, from the value coercions the assignability
// toleration otherwise admits.
func hasArityMismatch(d frontend.Diagnostic) bool {
	for _, r := range d.Related {
		if r.Code == 2849 || hasArityMismatch(r) {
			return true
		}
	}
	return false
}

// toleratedImplicitAny is the set of checker diagnostic codes bento admits by
// lowering the untyped form to a dynamic value instead of failing the build.
// Each one is a noImplicitAny report over a binding, parameter, variable,
// member, or inferred return whose resolved type is already `any`, so tolerating
// it costs no precision the checker had. The element-index implicit-any reports
// (7052, 7053) moved to toleratedDynamicMember once the dynamic member path grew
// to fold an absent element read; the remaining index-access and module-resolution
// reports (7015, 7016, 7017) are still deliberately absent and gate until their
// own slices land, staying attributable to them. Binding element (7031) is
// admitted now that the untyped destructured parameter lowers against a dynamic
// slot: the checker infers an anonymous object type from the pattern, and the
// lowerer gives such a parameter one boxed value.Value slot whose bound names read
// out through the dynamic Get protocol. A pattern shape the dynamic path does not
// serve yet (an array pattern, a rename, a default, a nested element) hands back
// cleanly rather than emitting broken Go, so tolerating the code never produces Go
// that does not compile.
//
// The circular self-reference reports (7022, 7023, 7024) are deliberately not
// here. They do not name an untyped form that resolves to `any`; they name a
// declaration whose type the checker could not compute at all because it
// references itself, `var a = { f: a }` being the small case. There is no dynamic
// slot that stands in for a type the checker never formed, so the honest outcome
// is a handback, not a lowering. Their absence gates every such case rather than
// emitting Go for a program TypeScript rejects outright.
var toleratedImplicitAny = map[int]bool{
	7005: true, // Variable 'X' implicitly has an 'any' type.
	7006: true, // Parameter 'X' implicitly has an 'any' type.
	7008: true, // Member 'X' implicitly has an 'any' type.
	7010: true, // 'X', which lacks return-type annotation, implicitly has an 'any' return type.
	7011: true, // Function expression, which lacks return-type annotation, implicitly has an 'any' return type.
	7018: true, // Object literal's property 'X' implicitly has an 'any' type.
	7019: true, // Rest parameter 'X' implicitly has an 'any[]' type.
	7031: true, // Binding element 'X' implicitly has an 'any' type.
	7032: true, // Property 'X' implicitly has type 'any', because its set accessor lacks a parameter type annotation.
	7033: true, // Property 'X' implicitly has type 'any', because its get accessor lacks a return type annotation.
	7034: true, // Variable 'X' implicitly has type 'any' in some locations where its type cannot be determined.
}

// toleratedDynamicMember is the set of checker diagnostic codes bento admits by
// resolving the member read at run time instead of failing the build. Each one
// reports that a property the source names is not on the receiver's static type.
// The read is not a build error for an AOT that runs the value model: on a
// receiver whose type interned to a Go struct the absent property is a provable
// miss and folds to undefined, and on any other receiver the lowerer hands the
// read back to a later slice. No lowering path emits a selector for a property
// the shape does not declare, so tolerating these codes never produces Go that
// fails to compile; it only lets a resolvable read through and leaves the rest a
// handback. The "Did you mean" variant (2551) is the same absent-property read
// with a spelling suggestion, so it tolerates on the same terms. The element
// forms (7053, 7052) are the bracket spelling of the same read: o["k"] with a
// string-literal key the shape does not declare folds to undefined exactly as o.k
// does, and o[k] with a computed key, which the shape cannot prove absent, hands
// back to the struct-to-value boxer slice rather than emitting a wrong read.
var toleratedDynamicMember = map[int]bool{
	2339: true, // Property 'X' does not exist on type 'Y'.
	2551: true, // Property 'X' does not exist on type 'Y'. Did you mean 'Z'?
	7053: true, // Element implicitly has an 'any' type because expression of type 'X' can't be used to index type 'Y'.
	7052: true, // Element implicitly has an 'any' type because type 'X' has no index signature.
}

// toleratedUnknown is the set of checker diagnostic codes bento admits by
// running the unknown-typed value through its dynamic value path instead of
// failing the build. Under strict checking the catch binding is typed `unknown`
// (useUnknownInCatchVariables, on with strict), so every `catch (e)` body that
// reads, calls, or otherwise uses the caught value draws one of these before the
// body can lower, the wall behind the whole throwing-test population. The lowerer
// already treats `unknown` exactly like `any`: isDynamic is true for both, so an
// unknown value flows as a boxed value.Value, a member read on it folds or hands
// back through the dynamic member path, a call goes through value.Call, and an
// operator takes the dynamic operand path. No lowering path emits a static
// selector or typed operation on an unknown value, so tolerating these codes
// never produces Go that fails to compile; it only lets the dynamic form through
// and leaves anything the dynamic path cannot yet model a handback. 18046 is the
// identifier form (`e` used directly) and 2571 the object-expression form (a
// member or call whose receiver expression is unknown); they tolerate on the
// same terms.
var toleratedUnknown = map[int]bool{
	2571:  true, // Object is of type 'unknown'.
	18046: true, // 'X' is of type 'unknown'.
}

// toleratedLooseForm is the set of checker diagnostic codes bento admits because
// the flagged form has valid run-time semantics the lowerer reproduces, the
// checker only warns that the source is looser than good style. 2695 reports a
// comma expression whose left side has no side effect ("Left side of comma
// operator is unused and has no side effects"): the runtime still evaluates the
// left, discards it, and yields the right, exactly what the comma lowering emits
// (the left runs into the blank identifier, the right is returned), so tolerating
// it lets a pure-left comma reach the renderer where a side-effecting comma
// already lowered. A comma the lowerer cannot yet model, a right operand outside
// the lowerable subset, still hands back, so admitting the code never produces Go
// that fails to compile.
var toleratedLooseForm = map[int]bool{
	2695: true, // Left side of comma operator is unused and has no side effects.
}

// toleratedArity is the set of checker diagnostic codes bento admits because a
// call whose argument count does not match the callee's declared arity still has
// defined run-time behavior the lowerer reproduces or safely refuses. JavaScript
// binds a missing argument to undefined and ignores an extra one, so 2554
// ("Expected N arguments, but got M") and 2555 ("Expected at least N arguments,
// but got M") mark a mismatch the language runs rather than a fault. The renderer
// fills a defaultless omission on a dynamic parameter with undefined and drops an
// extra argument that has no side effect; a defaultless static omission and a
// side-effecting extra each hand back to a later slice, so admitting the code
// lets the runnable calls through and never emits Go that fails to compile.
var toleratedArity = map[int]bool{
	2554: true, // Expected N arguments, but got M.
	2555: true, // Expected at least N arguments, but got M.
}

// toleratedArithOperand is the set of checker diagnostic codes bento admits
// because an arithmetic operator over a non-number primitive still has defined
// run-time behavior the lowerer reproduces. 2362 ("The left-hand side of an
// arithmetic operation must be of type 'any', 'number', 'bigint' or an enum
// type") and 2363 (its right-hand-side twin) flag a statically typed string or
// boolean used with -, *, /, %, **, or a bitwise operator. JavaScript coerces
// each such operand through ToNumber before the numeric operation, so the
// renderer lowers a string through value.StringToNumber and a boolean through
// value.BoolToNumber and applies the operator to the two float64 results. An
// operand that is not a number-coercible primitive, an object or a bigint mixed
// with a number, is not this case and still hands back, so admitting the code
// lets the runnable forms through and never emits Go that fails to compile.
var toleratedArithOperand = map[int]bool{
	2362: true, // The left-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.
	2363: true, // The right-hand side of an arithmetic operation must be of type 'any', 'number', 'bigint' or an enum type.
}

// toleratedDeleteForm is the set of checker diagnostic codes bento admits because
// a delete over a non-reference operand still has defined run-time behavior the
// lowerer reproduces. 2703 ("The operand of a 'delete' operator must be a property
// reference") flags delete over a literal, an arithmetic expression, or another
// value that is not a property reference. JavaScript evaluates such an operand and
// yields true without removing anything, exactly what the renderer emits: a
// side-effect-free operand folds to the constant true and a side-effecting one
// hands back to a later slice, so admitting the code lets the runnable forms
// through and never produces Go that fails to compile. The strict-mode
// identifier-delete error (1102) is a real early SyntaxError and is deliberately
// absent, so delete of a bare variable still gates.
var toleratedDeleteForm = map[int]bool{
	2703: true, // The operand of a 'delete' operator must be a property reference.
}

// toleratedComparison is the set of checker diagnostic codes bento admits because
// an equality comparison the checker judges pointless still has defined run-time
// behavior the lowerer reproduces. 2367 ("This comparison appears to be
// unintentional because the types 'X' and 'Y' have no overlap") flags == or != (and
// their strict twins) between operands whose static types the checker cannot see
// overlapping, the shape a literal-typed const or a mixed-primitive comparison
// takes: 1 == "1", "a" == "b", true == 1. JavaScript still runs the abstract
// equality comparison, so the renderer lowers a static primitive pair through
// value.LooseEquals (a mixed pair coercing, a string pair comparing by code unit)
// and a two-number or two-boolean pair through its native Go comparison. A pair the
// lowerer cannot yet model, an object operand with no dynamic box or a strict
// comparison outside the primitive set, still hands back, so admitting the code lets
// the runnable forms through and never emits Go that fails to compile.
var toleratedComparison = map[int]bool{
	2367: true, // This comparison appears to be unintentional because the types 'X' and 'Y' have no overlap.
}

// toleratedComputedKey is the set of checker diagnostic codes bento admits because
// an object literal's computed property name has a defined run-time key even when
// the checker rejects its static type. 2464 ("A computed property name must be of
// type 'string', 'number', 'symbol', or 'any'") flags a computed key `[expr]`
// whose expression is typed something outside that set, a boolean being the common
// case. JavaScript still evaluates the key and runs ToPropertyKey on it, turning a
// boolean into "true"/"false" and any other value into its string form, so an
// object literal carrying such a key is not statically fixed and lowers through the
// dynamic bag: the renderer boxes the literal and emits SetKeyed over the boxed key,
// whose SetElem runs the same ToString the language does. A key the renderer cannot
// box, an object-typed key, still hands back to a later slice, so admitting the code
// lets the runnable forms through and never emits Go that fails to compile.
var toleratedComputedKey = map[int]bool{
	2464: true, // A computed property name must be of type 'string', 'number', 'symbol', or 'any'.
}

// toleratedAssignability is the set of checker diagnostic codes bento admits because
// a value the checker calls not assignable to its slot still has defined run-time
// behavior the lowerer reproduces or safely refuses. 2345 ("Argument of type 'X' is
// not assignable to parameter of type 'Y'") flags a call argument, and 2322 ("Type 'X'
// is not assignable to type 'Y'") flags an assignment or initializer, whose value type
// the checker cannot see fitting the slot, the shape a widened literal (a number for a
// numeric-literal-union parameter, both float64) or a cross-primitive pass (a number
// for a string slot) takes.
//
// Two layers keep admitting the codes from ever emitting Go the toolchain rejects. The
// argument, constructor, and binding bridges hand back when a not-assignable value and
// its slot lower to different Go types and land the value directly when they lower to
// the same one, so a site the guard reaches either runs or routes to the interpreter.
// A site no guarded bridge reaches, a callback whose parameter type mismatches a builtin
// higher-order method's element, a value dropped into a builtin element slot, an
// assignment construct with no guard, is caught by the renderer's end-of-render
// reconciliation, which hands the whole unit back rather than ship the broken Go such a
// path would emit. So the representation-safe sites lower, the mismatched ones route to
// the engine where the run-time coercion still runs, and no admitted program compiles to
// Go that fails to build.
var toleratedAssignability = map[int]bool{
	2345: true, // Argument of type 'X' is not assignable to parameter of type 'Y'.
	2322: true, // Type 'X' is not assignable to type 'Y'.
}

// toleratedOverload is the set of diagnostic codes bento admits for a call the checker
// resolves against no overload signature. 2769 ("No overload matches this call") flags a
// call whose arguments fit none of a function's overload signatures, which at run time
// JavaScript still dispatches to the implementation with the arguments it was given. The
// lowerer reproduces that for a user-defined overloaded function by routing the call
// through the implementation's boxed dispatch, and its end-of-render reconciliation hands
// the unit back for any 2769 site that path did not reach (a mismatched builtin overload,
// a constructor), so admitting the code never emits Go the toolchain rejects.
var toleratedOverload = map[int]bool{
	2769: true, // No overload matches this call.
}

// toleratedIterable is the set of checker diagnostic codes bento admits because a
// spread or a for...of over a value the checker types as non-iterable still has
// defined run-time behavior the runtime reproduces or throws for. 2488 ("Type 'X'
// must have a '[Symbol.iterator]()' method that returns an iterator") flags a
// `[...x]`, a `for (const e of x)`, a spread argument, or a destructuring source
// whose static type is too weak for the checker to see an iterator on, the empty
// object type `{}` and a union carrying void being the common shapes. The type is a
// strictness artifact of composing JavaScript as TypeScript, not a test's own
// expectation: JavaScript looks the iterator up on the value at run time, walking it
// when it is iterable and throwing a TypeError when it is not, exactly the semantics
// the engine runs.
//
// Tolerating the code never emits Go that fails to compile, because a value the
// checker calls non-iterable can never reach a lowering path that emits an iteration.
// Every such path (the for...of loop, the array-literal spread, the spread into a
// rest parameter, and the array destructuring source) gates on the value being a
// provable array, string, typed array, or a type carrying a [Symbol.iterator], and a
// type that satisfies any of those is one the checker already sees as iterable, so it
// would not draw 2488 in the first place. The two sets are disjoint by construction:
// a 2488 value always hands back to the engine rather than lowering, and the engine
// runs the same run-time iterator lookup the language does, so it never turns a wrong
// result green and a genuinely non-iterable value still throws its TypeError there.
// The array-type and downlevel-iteration variants (2549, 2569) are the same lookup
// under a different message and stay deliberately absent until their own slice
// measures them, so admitting 2488 alone stays attributable to this wave.
var toleratedIterable = map[int]bool{
	2488: true, // Type 'X' must have a '[Symbol.iterator]()' method that returns an iterator.
}

// toleratedImplicitThis is the set of checker diagnostic codes bento admits
// because a `this` read outside any typed receiver still has defined run-time
// behavior the lowerer refuses cleanly rather than mislowers. 2683 ("'this'
// implicitly has type 'any' because it does not have a type annotation") flags a
// `this` used inside a plain function that carries no `this` parameter, the shape
// a test262 harness helper written as a free function takes. The read is a
// strictness artifact of composing JavaScript as TypeScript, not a test's own
// expectation: at run time the function's `this` binds to the call-site receiver
// or undefined, and the checker only warns that it cannot name that type.
//
// Tolerating the code never emits Go that fails to compile, because a `this` the
// checker calls implicitly-any can never reach a lowering path that emits a
// receiver. The renderer lowers `this` only inside a class body it is currently
// lowering, where thisName names the receiver; a `this` anywhere else, at the top
// level or inside a plain function nested in a method, finds thisName empty and
// hands back to a later slice (expr.go), and a plain function declaration nested
// in a body is itself a handback before its `this` is ever reached. So a 2683
// value always routes to the engine rather than lowering, the engine binds `this`
// with the same run-time rule the language does, and admitting the code never
// turns a wrong result green.
var toleratedImplicitThis = map[int]bool{
	2683: true, // 'this' implicitly has type 'any' because it does not have a type annotation.
}

// toleratedConstructAny is the set of checker diagnostic codes bento admits
// because a `new` over a target the checker cannot see a construct signature on
// still has defined run-time behavior the engine reproduces. 7009 ("'new'
// expression, whose target lacks a construct signature, implicitly has an 'any'
// type") flags `new f()` where f is typed as a plain function rather than a
// constructor, the shape a test262 harness helper that constructs through a plain
// callable takes. The report is a strictness artifact of composing JavaScript as
// TypeScript, not a test's own expectation: JavaScript builds a fresh object with
// f as its constructor and runs f's body over it, so the expression has a defined
// value the checker only warns it cannot name a type for.
//
// Tolerating the code never emits Go that fails to compile, because a target the
// checker calls construct-signature-less can never reach a lowering path that emits
// a construction. The renderer lowers a `new` only for a target it recognizes as a
// user class (through the class symbol) or a named built-in constructor, each of
// which carries a construct signature and so never draws 7009; a `new` over a plain
// function falls through to the generic "new of a constructor other than a built-in
// error is a later slice" hand-back. So a 7009 target always routes to the engine
// rather than lowering, the engine runs the same run-time construct the language
// does, and admitting the code never turns a wrong result green.
var toleratedConstructAny = map[int]bool{
	7009: true, // 'new' expression, whose target lacks a construct signature, implicitly has an 'any' type.
}

// toleratedPossiblyUndefined is the set of checker diagnostic codes bento admits
// because a value the checker cannot prove is defined still has defined run-time
// behavior the lowerer either reproduces or refuses cleanly. 18048 ("'X' is
// possibly 'undefined'") and 2532 ("Object is possibly 'undefined'") flag a read,
// call, member access, or arithmetic on a value typed `T | undefined` at a site
// the checker will not narrow, because the guard that proves it present is
// implicit or lives in a helper the checker cannot follow. The report is a
// strictness artifact of composing JavaScript as TypeScript, not a test's own
// expectation: at run time the value is the present T (the test knows the map has
// the key, the regex matches, the pop is non-empty), so the operation runs, and on
// the genuinely-absent case the language throws a TypeError the engine also throws.
//
// Tolerating the code never emits Go that fails to compile or turns a wrong result
// green, because a value the checker calls possibly-undefined is lowered only where
// its `T | undefined` type already forces a representation the operation is sound
// over. A binding of that type interns to value.Opt[T] (optionals.go), whose read
// outside a narrowing unwraps through .Get(); a member access or arithmetic on the
// un-narrowed optional finds no static field or numeric operand and hands back to
// the engine rather than mislowering. So a 18048/2532 value either lowers through
// the Opt path faithful on both the present and absent cases, or routes to the
// engine that runs the same run-time rule the language does; neither path reads a
// wrong value out of the absent case.
var toleratedPossiblyUndefined = map[int]bool{
	18048: true, // 'X' is possibly 'undefined'.
	2532:  true, // Object is possibly 'undefined'.
}

// gateCgo detects whether any go: import the program reached pulls in cgo and
// decides what the build does about it (section 9.5). It returns whether the link
// must enable cgo. A cgo import is allowed to proceed only on an explicit
// acknowledgment, allowCgo or CGO_ENABLED=1 in the environment, since taking cgo
// forfeits the zero-cgo cross-compile bento otherwise guarantees; without that
// acknowledgment it returns the section 9.5 diagnostic naming the cgo library and
// the import that pulled it in, so the loss is never silent. A detector that
// cannot run degrades to not-cgo rather than blocking the build, since a real cgo
// import would fail the go build that follows with its own error.
func gateCgo(goPaths []string, allowCgo bool) (bool, error) {
	for _, path := range goPaths {
		use, ok, err := goimport.DetectCgo(path)
		if err != nil || !ok {
			continue
		}
		if allowCgo || os.Getenv("CGO_ENABLED") == "1" {
			return true, nil
		}
		return false, fmt.Errorf("bento build: %s", cgoDiagnostic(use))
	}
	return false, nil
}

// cgoDiagnostic is the section 9.5 message for a go: import that pulls in cgo: it
// names the cgo library and the import that reached it, states the build has left
// the zero-cgo path, says how to proceed, and points at a pure-Go alternative when
// bento knows one. It is written as a human would explain the tradeoff, since this
// is the moment an author learns their build no longer cross-compiles the easy way.
func cgoDiagnostic(use goimport.CgoUse) string {
	var b strings.Builder
	b.WriteString("a go: import requires cgo, which breaks the zero-cgo cross-compile\n")
	if use.Cgo == use.Import {
		fmt.Fprintf(&b, "  go:%s uses cgo\n", use.Import)
	} else {
		fmt.Fprintf(&b, "  go:%s pulls in cgo through %s\n", use.Import, use.Cgo)
	}
	b.WriteString("  the default build sets CGO_ENABLED=0, so it cannot include this import\n")
	b.WriteString("  to build anyway, pass --allow-cgo (or set CGO_ENABLED=1) with a C toolchain on the host\n")
	if alt := goimport.PureGoAlternative(use.Cgo); alt != "" {
		fmt.Fprintf(&b, "  for a pure-Go alternative, use %s\n", alt)
	} else if alt := goimport.PureGoAlternative(use.Import); alt != "" {
		fmt.Fprintf(&b, "  for a pure-Go alternative, use %s\n", alt)
	}
	b.WriteString("  the zero-cgo cross-compile guarantee holds only for pure-Go imports")
	return b.String()
}

// link writes the generated Go program into a scratch directory inside the bento
// module tree and compiles it to output with `go build`. Building inside the
// module tree is what lets the program import bento's value package and its
// transitive dependencies with no separate go.mod, go.sum, or module download,
// so a build is hermetic and offline. cgo selects CGO_ENABLED for the go build:
// off by default, which keeps a pure-Go build cross-compilable and turns any
// accidental cgo dependency into a build error, and on for a build that
// acknowledged a cgo import so the toolchain can link it.
func link(source, output string, cgo bool) error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(root, "bento-build-")
	if err != nil {
		return fmt.Errorf("bento build: scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		return fmt.Errorf("bento build: write generated source: %w", err)
	}

	cmd := exec.Command("go", "build", "-o", output, ".")
	cmd.Dir = dir
	enabled := "0"
	if cgo {
		enabled = "1"
	}
	cmd.Env = append(os.Environ(), "CGO_ENABLED="+enabled)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bento build: go build failed: %w\n%s", err, stderr.String())
	}
	return nil
}

// moduleRoot finds the root of the bento module the running binary was built
// from, the directory whose go.mod declares module github.com/tamnd/bento. The
// generated program is compiled there so its import of the value package
// resolves locally. BENTO_MODULE_ROOT overrides the search, which is how a CI
// job that builds bento outside its checkout (so the binary is not under the
// module tree) points the AOT build at the source it already has.
func moduleRoot() (string, error) {
	if r := os.Getenv("BENTO_MODULE_ROOT"); r != "" {
		if isModuleRoot(r) {
			return r, nil
		}
		return "", fmt.Errorf("bento build: BENTO_MODULE_ROOT=%s does not hold the bento module (no go.mod for github.com/tamnd/bento)", r)
	}
	var starts []string
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, start := range starts {
		if root, ok := findModuleRoot(start); ok {
			return root, nil
		}
	}
	return "", fmt.Errorf("bento build: cannot locate the bento module source to link the runtime; " +
		"set BENTO_MODULE_ROOT to a checkout of github.com/tamnd/bento")
}

// findModuleRoot walks up from start looking for the bento module root.
func findModuleRoot(start string) (string, bool) {
	dir := start
	for {
		if isModuleRoot(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// isModuleRoot reports whether dir holds the go.mod that declares the bento
// module. It matches the module line exactly so a nested module that happens to
// sit above the binary is not mistaken for it.
func isModuleRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "module github.com/tamnd/bento" {
			return true
		}
	}
	return false
}
