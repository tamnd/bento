// Package conformance is the fixture corpus for the ahead-of-time lowerer. Each
// fixture is a directory under fixtures/, named NNNN_short_name, that pins one
// slice of what pkg/lower emits: the Go it generates for a TypeScript program, the
// output that Go produces when it runs, or the hand-back reason a construct outside
// the lowerable subset returns. The layout and the three checks are described in
// notes/Spec/2075/lower/14_conformance.md; this file is the discovery half, the
// harness_test.go beside it is the running half.
//
// The corpus is found by walking the tree, not by a registration list, so adding a
// fixture is only a matter of dropping a directory in. That is the property that
// lets each new feature bring its own fixtures without editing a central table.
package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// idPattern matches a fixture directory: four digits, an underscore, a snake_case
// handle. The digits band the fixture by subject (see the band table in the index
// doc) and order the listing; the handle is what a human reads in test output.
var idPattern = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)$`)

// Fixture is one corpus entry, resolved to absolute paths. Golden and Oracle hold
// the canonical path a fixture's files live at whether or not they exist yet, so
// `go test -update` can write a golden that is not there. HasGolden and HasOracle
// say whether the file is present, which is how a fixture opts into the checks it
// makes: an emission-only fixture has no oracle, a handback fixture has neither.
type Fixture struct {
	ID        string // the four-digit id, e.g. "0308"
	Name      string // the handle, e.g. "math_hypot"
	Slug      string // the directory name, e.g. "0308_math_hypot"
	Dir       string // absolute path to the fixture directory
	Input     string // absolute path to input.ts (always present)
	Golden    string // canonical path to emit.golden, present or not
	Oracle    string // canonical path to oracle.txt, present or not
	HasGolden bool   // emit.golden exists on disk
	HasOracle bool   // oracle.txt exists on disk
	Meta      Meta   // parsed fixture.toml, zero value if absent
}

// Meta is the optional per-fixture metadata from fixture.toml. Feature is the
// dispatch surface the fixture exercises, the tag that lets an incremental run
// select only the fixtures a change touches. Handback, when set, marks the fixture
// as a boundary case: lowering must return a NotYetLowerable whose reason contains
// this text and emit no Go. Skip, when set, records why the fixture is parked.
type Meta struct {
	Feature  string
	Handback string
	Skip     string
}

// Discover walks root and returns every fixture it finds, sorted by id so the
// suite runs in a stable order. A directory whose name matches the id pattern is a
// fixture; anything else is ignored, so notes and scratch files can sit in the tree
// without being mistaken for a case.
func Discover(root string) ([]Fixture, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve fixtures root: %w", err)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("read fixtures root: %w", err)
	}
	var fixtures []Fixture
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := idPattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		dir := filepath.Join(abs, e.Name())
		f := Fixture{
			ID:     m[1],
			Name:   m[2],
			Slug:   e.Name(),
			Dir:    dir,
			Input:  filepath.Join(dir, "input.ts"),
			Golden: filepath.Join(dir, "emit.golden"),
			Oracle: filepath.Join(dir, "oracle.txt"),
		}
		f.HasGolden = fileExists(f.Golden)
		f.HasOracle = fileExists(f.Oracle)
		if p := filepath.Join(dir, "fixture.toml"); fileExists(p) {
			meta, err := readMeta(p)
			if err != nil {
				return nil, fmt.Errorf("fixture %s: %w", e.Name(), err)
			}
			f.Meta = meta
		}
		fixtures = append(fixtures, f)
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].Slug < fixtures[j].Slug })
	return fixtures, nil
}

// fileExists reports whether path is a readable file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// readMeta parses a fixture.toml. It is a deliberately small subset of TOML: blank
// lines, # comments, and top-level `key = "value"` pairs. The fixtures need nothing
// richer, and keeping the parser in-tree keeps the corpus free of a dependency.
func readMeta(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return Meta{}, fmt.Errorf("line %d: not a key = value pair: %q", i+1, raw)
		}
		key = strings.TrimSpace(key)
		val, err := unquote(strings.TrimSpace(val))
		if err != nil {
			return Meta{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		switch key {
		case "feature":
			m.Feature = val
		case "handback":
			m.Handback = val
		case "skip":
			m.Skip = val
		default:
			return Meta{}, fmt.Errorf("line %d: unknown key %q", i+1, key)
		}
	}
	return m, nil
}

// unquote strips the surrounding double quotes from a fixture.toml value. A bare
// value without quotes is an error, so the format stays one obvious shape.
func unquote(s string) (string, error) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strconv.Unquote(s)
	}
	return "", fmt.Errorf("value must be double-quoted: %q", s)
}

// Oracle is the expected result of running a fixture's program, parsed from its
// oracle.txt. The file is framed into sections by `== name:` headers, matching the
// format in the conformance doc and unagi's oracle files. A missing exit section
// defaults to 0; a missing stdout section defaults to empty; an empty exception
// section means the program is expected to run to completion.
type Oracle struct {
	Exit      int
	Stdout    string
	Exception string
}

// oracleHeader matches a section header line, `== name:` with optional surrounding
// space, capturing the section name.
var oracleHeader = regexp.MustCompile(`^==\s*([a-z]+)\s*:\s*$`)

// ParseOracle reads the framed sections of an oracle.txt. Section content is the
// lines between one header and the next, with a single trailing newline trimmed so
// a one-line expected value does not have to fight the file's own final newline.
// An unknown section name or a malformed exit value is an error, so a typo in a
// fixture surfaces as a failure rather than a silently ignored expectation.
func ParseOracle(content string) (Oracle, error) {
	o := Oracle{Exit: 0}
	var (
		section         string
		buf             []string
		haveExitSection bool
		rawExit         string
	)
	flush := func() error {
		if section == "" {
			return nil
		}
		body := strings.Join(buf, "\n")
		body = strings.TrimSuffix(body, "\n")
		switch section {
		case "stdout":
			o.Stdout = body
		case "exception":
			o.Exception = strings.TrimSpace(body)
		case "exit":
			haveExitSection = true
			rawExit = strings.TrimSpace(body)
		default:
			return fmt.Errorf("unknown oracle section %q", section)
		}
		return nil
	}
	for line := range strings.SplitSeq(content, "\n") {
		if m := oracleHeader.FindStringSubmatch(line); m != nil {
			if err := flush(); err != nil {
				return Oracle{}, err
			}
			section = m[1]
			buf = buf[:0]
			continue
		}
		if section != "" {
			buf = append(buf, line)
		}
	}
	if err := flush(); err != nil {
		return Oracle{}, err
	}
	if haveExitSection {
		code, err := strconv.Atoi(rawExit)
		if err != nil {
			return Oracle{}, fmt.Errorf("exit section is not an integer: %q", rawExit)
		}
		o.Exit = code
	}
	return o, nil
}
