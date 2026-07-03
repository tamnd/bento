package goimport

import (
	"strings"
	"testing"
)

// These tests load real standard-library packages, so they exercise the whole
// path go/packages, go/types, doc extraction, and the generator against genuine
// Go source rather than a fixture. The standard library is always present, so the
// tests are hermetic and need no network.

func TestLoadCryptoSHA256(t *testing.T) {
	dts, err := Load("crypto/sha256", "v1.0.0")
	if err != nil {
		t.Fatalf("Load(crypto/sha256): %v", err)
	}
	if !strings.Contains(dts, "// Generated from crypto/sha256@v1.0.0 by bento.") {
		t.Errorf("missing header banner:\n%s", dts)
	}
	// Sum256(data []byte) [32]byte: the []byte parameter is a Uint8Array.
	if !strings.Contains(dts, "export function Sum256(data: Uint8Array)") {
		t.Errorf("missing Sum256 declaration:\n%s", dts)
	}
	// The Size and BlockSize constants project to number.
	if !strings.Contains(dts, "export const Size: number;") {
		t.Errorf("missing Size constant:\n%s", dts)
	}
	// crypto/sha256's exported functions are documented, so a TSDoc block is present.
	if !strings.Contains(dts, "/**") {
		t.Errorf("expected doc comments in the output:\n%s", dts)
	}
}

func TestLoadHashInterface(t *testing.T) {
	dts, err := Load("hash", "")
	if err != nil {
		t.Fatalf("Load(hash): %v", err)
	}
	if !strings.Contains(dts, "export interface Hash {") {
		t.Errorf("hash.Hash did not project to an interface:\n%s", dts)
	}
	// Sum(b []byte) []byte projects to (b: Uint8Array): Uint8Array.
	if !strings.Contains(dts, "Sum(b: Uint8Array): Uint8Array;") {
		t.Errorf("hash.Hash.Sum did not project as expected:\n%s", dts)
	}
	// Write is embedded from io.Writer and returns (int, error), which throws.
	if !strings.Contains(dts, "Write(p: Uint8Array): number;") {
		t.Errorf("embedded io.Writer.Write did not project as expected:\n%s", dts)
	}
}

func TestLoadHeaderWithoutVersion(t *testing.T) {
	dts, err := Load("io", "")
	if err != nil {
		t.Fatalf("Load(io): %v", err)
	}
	if !strings.Contains(dts, "// Generated from io by bento.") {
		t.Errorf("header should omit the version suffix when unpinned:\n%s", firstLines(dts, 3))
	}
}

func TestLoadUnknownPackageErrors(t *testing.T) {
	_, err := Load("example.com/definitely/not/a/real/package", "")
	if err == nil {
		t.Fatal("loading a nonexistent package should error")
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
