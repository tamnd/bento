package resolve

import "strings"

// ResolveError is a resolution failure carrying a Node-matching error code so a
// program that catches on err.code catches the same code on bento as on Node.
type ResolveError struct {
	// Code is the Node error code, such as ERR_MODULE_NOT_FOUND.
	Code string
	// Specifier is the import that failed.
	Specifier string
	// Importer is the module that requested the import, "" for an entry point.
	Importer string
	// Searched lists the directories tried, for a not-found error.
	Searched []string
	// Message is the human-readable explanation.
	Message string
}

func (e *ResolveError) Error() string {
	var b strings.Builder
	if e.Message != "" {
		b.WriteString(e.Message)
	} else {
		b.WriteString("cannot resolve " + e.Specifier)
	}
	if e.Importer != "" {
		b.WriteString(" from " + e.Importer)
	}
	if len(e.Searched) > 0 {
		b.WriteString("\nsearched:")
		for _, dir := range e.Searched {
			b.WriteString("\n  " + dir)
		}
	}
	return b.String()
}

// notFound builds an ERR_MODULE_NOT_FOUND error for a specifier.
func notFound(specifier string, parent *Module, searched []string) *ResolveError {
	return &ResolveError{
		Code:      "ERR_MODULE_NOT_FOUND",
		Specifier: specifier,
		Importer:  importerPath(parent),
		Searched:  searched,
		Message:   "cannot find module " + specifier,
	}
}
