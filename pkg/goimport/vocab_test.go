package goimport

import (
	"strings"
	"testing"
)

// TestVocabularyDeclaresEveryHelper is the guard that keeps the generator and the
// vocabulary module from drifting: every helper name the Mapper can emit into an
// import must have an actual declaration in bento:go, or a generated .d.ts would
// import a name that does not exist and fail to type-check.
func TestVocabularyDeclaresEveryHelper(t *testing.T) {
	v := Vocabulary()
	for _, h := range helperOrder {
		name := string(h)
		if !declaresName(v, name) {
			t.Errorf("bento:go does not declare the helper %q the Mapper can emit", name)
		}
	}
}

// declaresName reports whether the vocabulary declares an exported symbol of the
// given name, in any of the forms the module uses.
func declaresName(vocab, name string) bool {
	for _, prefix := range []string{
		"export interface " + name,
		"export type " + name,
		"export declare class " + name,
		"export declare function " + name,
	} {
		if strings.Contains(vocab, prefix) {
			return true
		}
	}
	return false
}

// TestGeneratedImportResolvesAgainstVocabulary checks the round trip end to end:
// generate a declaration file that references a helper, then confirm every name it
// imports from bento:go is declared by the vocabulary. This is the property that
// actually matters, that a generated file's imports all resolve.
func TestGeneratedImportResolvesAgainstVocabulary(t *testing.T) {
	src := `package p
import "io"
import "context"
func Do(ctx context.Context, r io.Reader) (int, error) { return 0, nil }
func Stream() chan int { return nil }
`
	pkg := checkSource(t, src)
	dts := Generate(pkg, GenOptions{})
	v := Vocabulary()
	for _, name := range importedHelperNames(t, dts) {
		if !declaresName(v, name) {
			t.Errorf("generated .d.ts imports %q, which bento:go does not declare", name)
		}
	}
}

// importedHelperNames pulls the names out of the single bento:go import line of a
// generated declaration file, so the test can check each against the vocabulary.
func importedHelperNames(t *testing.T, dts string) []string {
	t.Helper()
	const marker = `import type { `
	_, rest, ok := strings.Cut(dts, marker)
	if !ok {
		t.Fatalf("generated file has no bento:go import to check:\n%s", dts)
	}
	inside, _, ok := strings.Cut(rest, " }")
	if !ok {
		t.Fatalf("malformed import line:\n%s", dts)
	}
	names := strings.Split(inside, ", ")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	return names
}

func TestVocabularyModuleSpecifier(t *testing.T) {
	if VocabularyModule != "bento:go" {
		t.Errorf("vocabulary module specifier = %q, want bento:go", VocabularyModule)
	}
}
