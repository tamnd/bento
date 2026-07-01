package resolve

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// packageJSON is the subset of package.json the resolver interprets. exports and
// imports are parsed into an ordered tree, never a Go map, because conditional
// resolution picks the first matching condition in author key order.
type packageJSON struct {
	Name    string
	Version string
	Type    string
	Main    string
	Module  string
	Browser string
	Exports *exportsNode
	Imports *exportsNode
	dir     string
}

// nodeKind tags what shape an exports/imports node holds.
type nodeKind int

const (
	nodeString nodeKind = iota
	nodeMap
	nodeArray
	nodeNull
)

// exportsNode is one node in an ordered exports or imports tree. A map node
// keeps its entries in source order so condition selection is deterministic.
type exportsNode struct {
	kind    nodeKind
	str     string
	entries []exportsEntry
	array   []*exportsNode
}

// exportsEntry is one ordered key/value pair in a map node. The key is either a
// subpath (starts with ".") or "#..." for imports, or a condition name.
type exportsEntry struct {
	key   string
	value *exportsNode
}

// readPackageJSON reads and parses a package.json, caching by path since the
// nearest-package walk revisits the same files constantly. A missing file is a
// nil package and a nil error; a malformed one is an error.
func (r *Resolver) readPackageJSON(path string) (*packageJSON, error) {
	if hit, ok := r.pkgCache.Load(path); ok {
		entry := hit.(pkgCacheEntry)
		return entry.pkg, entry.err
	}
	pkg, err := r.parsePackageJSON(path)
	r.pkgCache.Store(path, pkgCacheEntry{pkg: pkg, err: err})
	return pkg, err
}

type pkgCacheEntry struct {
	pkg *packageJSON
	err error
}

func (r *Resolver) parsePackageJSON(path string) (*packageJSON, error) {
	if !r.fileExists(path) {
		return nil, nil
	}
	data, err := r.fs.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	var head struct {
		Name    string          `json:"name"`
		Version string          `json:"version"`
		Type    string          `json:"type"`
		Main    string          `json:"main"`
		Module  string          `json:"module"`
		Browser json.RawMessage `json:"browser"`
		Exports json.RawMessage `json:"exports"`
		Imports json.RawMessage `json:"imports"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("resolve: parse %s: %w", path, err)
	}

	pkg := &packageJSON{
		Name:    head.Name,
		Version: head.Version,
		Type:    head.Type,
		Main:    head.Main,
		Module:  head.Module,
		dir:     dirOf(path),
	}
	// browser may be a string (alternate main) or an object (remap); the string
	// form is all the resolver needs here.
	if len(head.Browser) > 0 && head.Browser[0] == '"' {
		_ = json.Unmarshal(head.Browser, &pkg.Browser)
	}
	if len(head.Exports) > 0 {
		node, err := parseExportsNode(head.Exports)
		if err != nil {
			return nil, fmt.Errorf("resolve: parse exports in %s: %w", path, err)
		}
		pkg.Exports = node
	}
	if len(head.Imports) > 0 {
		node, err := parseExportsNode(head.Imports)
		if err != nil {
			return nil, fmt.Errorf("resolve: parse imports in %s: %w", path, err)
		}
		pkg.Imports = node
	}
	return pkg, nil
}

// parseExportsNode walks a raw exports/imports value into an ordered tree using
// a token stream so map key order survives, which a map[string]any would drop.
func parseExportsNode(raw json.RawMessage) (*exportsNode, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	switch trimmed[0] {
	case 'n': // null
		return &exportsNode{kind: nodeNull}, nil
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		return &exportsNode{kind: nodeString, str: s}, nil
	case '[':
		return parseExportsArray(trimmed)
	case '{':
		return parseExportsMap(trimmed)
	default:
		return nil, fmt.Errorf("unexpected exports token %q", trimmed[0])
	}
}

func parseExportsArray(raw json.RawMessage) (*exportsNode, error) {
	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil, err
	}
	node := &exportsNode{kind: nodeArray}
	for _, item := range rawItems {
		child, err := parseExportsNode(item)
		if err != nil {
			return nil, err
		}
		node.array = append(node.array, child)
	}
	return node, nil
}

// parseExportsMap decodes an object while preserving key order by reading the
// JSON token stream directly instead of unmarshalling into a map.
func parseExportsMap(raw json.RawMessage) (*exportsNode, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	// Consume the opening brace.
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	node := &exportsNode{kind: nodeMap}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("exports object key is not a string")
		}
		var rawVal json.RawMessage
		if err := dec.Decode(&rawVal); err != nil {
			return nil, err
		}
		child, err := parseExportsNode(rawVal)
		if err != nil {
			return nil, err
		}
		node.entries = append(node.entries, exportsEntry{key: key, value: child})
	}
	return node, nil
}
