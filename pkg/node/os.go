package node

import "github.com/tamnd/bento/pkg/nodehost"

// osHostFuncs exposes the single os snapshot call. The snapshot itself, and the
// platform, arch, cpu, and network detail it gathers, live in the leaf pkg/nodehost
// package so the interpreter here and the AOT path share one implementation; this
// side only marshals it across the host bridge as the JSON string the os.js factory
// parses.
func osHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_os_info": func(_ []any) (any, error) { return nodehost.OSInfoJSON(), nil },
	}
}
