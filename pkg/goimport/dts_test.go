package goimport

import (
	"go/types"
	"strings"
	"testing"
)

// contains fails the test unless the generated .d.ts holds the given line, which
// keeps the assertions about the shape of one declaration rather than the whole
// file byte-for-byte, so unrelated additions do not churn the tests.
func contains(t *testing.T, dts, want string) {
	t.Helper()
	if !strings.Contains(dts, want) {
		t.Errorf("generated .d.ts missing %q\n---\n%s", want, dts)
	}
}

func TestGenerateFunctionThrowsError(t *testing.T) {
	src := `package zstd
func NewReader(r int) (int, error) { return 0, nil }
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{ImportPath: "example.com/zstd", Version: "v1.0.0"})
	contains(t, dts, "// Generated from example.com/zstd@v1.0.0 by bento.")
	// (int, error) hoists the error to a throw, so the result is just number.
	contains(t, dts, "export function NewReader(r: number): number;")
}

func TestGenerateStructInterfaceWithMethods(t *testing.T) {
	src := `package geo
type Point struct {
	X float64
	Y float64
	hidden int
}
func (p Point) Dist() float64 { return 0 }
func (p *Point) Scale(f float64) {}
func (p Point) unexported() {}
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{ImportPath: "example.com/geo"})
	contains(t, dts, "export interface Point {")
	contains(t, dts, "  X: number;")
	contains(t, dts, "  Y: number;")
	contains(t, dts, "  Dist(): number;")
	contains(t, dts, "  Scale(f: number): void;")
	// The unexported field and method never appear.
	if strings.Contains(dts, "hidden") || strings.Contains(dts, "unexported") {
		t.Errorf("unexported members leaked into the .d.ts:\n%s", dts)
	}
}

func TestGenerateOpaqueStruct(t *testing.T) {
	// A struct with no exported fields and no exported methods has no shape the author
	// can read or call, so it projects as an opaque token rather than an empty
	// interface (section 6.13).
	src := `package zstd
type DOption struct {
	level int
}
func (o DOption) apply() {}
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{ImportPath: "example.com/zstd"})
	contains(t, dts, `export type DOption = GoOpaque<"zstd.DOption">;`)
	if strings.Contains(dts, "export interface DOption") {
		t.Errorf("a field-free method-free struct projected as an interface, not an opaque token:\n%s", dts)
	}
}

func TestGenerateNamedInterface(t *testing.T) {
	src := `package p
type Reader interface {
	Read(buf []byte) (int, error)
}
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	contains(t, dts, "export interface Reader {")
	contains(t, dts, "  Read(buf: Uint8Array): number;")
}

func TestGenerateBrandedDefinedType(t *testing.T) {
	src := `package time
type Duration int64
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	contains(t, dts, `export type Duration = number & { readonly __brand: "time.Duration" };`)
}

func TestGenerateConstAndVar(t *testing.T) {
	src := `package p
const MaxSize = 1024
var Version = "1.0.0"
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	contains(t, dts, "export const MaxSize: number;")
	contains(t, dts, "export const Version: string;")
}

func TestGenerateHelperImport(t *testing.T) {
	src := `package p
import "io"
func Copy(dst io.Writer, src io.Reader) (int, error) { return 0, nil }
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	// The file references GoWriter and GoReader, so exactly those import.
	contains(t, dts, `import type { GoReader, GoWriter } from "bento:go";`)
	contains(t, dts, "export function Copy(dst: GoWriter, src: GoReader): number;")
}

func TestGenerateNoHelperImportWhenUnused(t *testing.T) {
	src := `package p
func Add(a int, b int) int { return a + b }
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	if strings.Contains(dts, "bento:go") {
		t.Errorf("a file that uses no helpers should not import bento:go:\n%s", dts)
	}
	contains(t, dts, "export function Add(a: number, b: number): number;")
}

func TestGenerateGenericFunction(t *testing.T) {
	src := `package p
func Map[T any, U any](s []T, f func(x T) U) []U { return nil }
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	contains(t, dts, "export function Map<T, U>(s: T[], f: (x: T) => U): U[];")
}

func TestGenerateDocComments(t *testing.T) {
	src := `package p
func NewReader(r int) int { return r }
`
	pkg := checkSource(t, src)
	docs := DocLookup(func(obj types.Object) string {
		if obj.Name() == "NewReader" {
			return "NewReader returns a new decoder that decompresses from r."
		}
		return ""
	})
	dts := Generate(pkg, GenOptions{Docs: docs})
	contains(t, dts, "/** NewReader returns a new decoder that decompresses from r. */")
	contains(t, dts, "export function NewReader(r: number): number;")
}

func TestGenerateSkipsUnexported(t *testing.T) {
	src := `package p
func Public() {}
func private() {}
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	contains(t, dts, "export function Public(): void;")
	if strings.Contains(dts, "private") {
		t.Errorf("unexported function leaked into the .d.ts:\n%s", dts)
	}
}
