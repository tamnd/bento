package node

import "github.com/tamnd/bento/pkg/nodehost"

// weburl backs the WHATWG URL global. The parsing itself lives in pkg/nodehost so
// the AOT path and the interpreter share one implementation; this file only binds
// the host callee. There is no I/O, so it registers alongside the pure host
// functions rather than through the loop-aware net install.

// urlHostFuncs returns the URL parsing host function. It is pure (no loop, no
// I/O), so HostFuncs bundles it with fs and os.
func urlHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_url_parse": func(args []any) (any, error) {
			return nodehost.URLParseJSON(str(args, 0), str(args, 1)), nil
		},
	}
}
