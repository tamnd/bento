package node

import (
	"context"
	"net"
	"sort"

	"github.com/tamnd/bento/pkg/engine"
)

// dnsBridgeState backs node:dns. Unlike net and http it owns no long-lived
// connections; each call is a one-shot lookup. JavaScript mints a request id,
// calls a host function with it, and Go runs the resolver on a pool goroutine
// and dispatches the result or error back keyed by that id.
//
// The two faces of Node's dns module map onto the two behaviors of Go's
// net.Resolver. dns.lookup is OS name resolution (getaddrinfo semantics) and
// uses the default resolver's host lookup; the resolveX family is raw DNS and
// uses the typed LookupX methods. bento keeps the pure-Go resolver path so the
// runtime stays cgo-free.
type dnsBridgeState struct {
	netBridge
	resolver *net.Resolver
}

func installDNS(eng engine.Engine, loop LoopHost) error {
	d := &dnsBridgeState{
		netBridge: netBridge{eng: eng, loop: loop},
		resolver:  &net.Resolver{},
	}
	for name, fn := range d.hostFuncs() {
		if err := eng.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

func (d *dnsBridgeState) hostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_dns_lookup":       d.lookup,
		"__bento_dns_resolve4":     d.resolve4,
		"__bento_dns_resolve6":     d.resolve6,
		"__bento_dns_resolveMx":    d.resolveMx,
		"__bento_dns_resolveTxt":   d.resolveTxt,
		"__bento_dns_resolveSrv":   d.resolveSrv,
		"__bento_dns_resolveNs":    d.resolveNs,
		"__bento_dns_resolveCname": d.resolveCname,
		"__bento_dns_resolvePtr":   d.reverse,
		"__bento_dns_reverse":      d.reverse,
	}
}

// run wraps the common lifecycle of one lookup: hold the loop open, do the
// blocking resolution on a pool goroutine, then dispatch back and release the
// loop. AddRef runs here on the loop goroutine (host functions always do), and
// Unref is posted from the pool goroutine. task returns a JSON-marshalable value
// on success or an error whose code Node would report.
func (d *dnsBridgeState) run(id int64, task func() (any, error)) {
	d.loop.AddRef()
	d.pool(func() {
		result, err := task()
		// Dispatch before releasing the loop reference. Unref is posted after the
		// dispatch so the loop cannot see the refcount hit zero and exit before it
		// runs the result callback.
		if err != nil {
			d.emit("__bento_dns_dispatchError", id, dnsCode(err), err.Error())
		} else {
			d.emit("__bento_dns_dispatchResult", id, jsonString(result))
		}
		d.loop.Post(func() { d.loop.Unref() })
	})
}

// dnsAddr is one address in a lookup result, matching Node's {address, family}.
type dnsAddr struct {
	Address string `json:"address"`
	Family  int    `json:"family"`
}

func (d *dnsBridgeState) lookup(args []any) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	family := intArg(args, 2)
	all := intArg(args, 3) != 0
	d.run(id, func() (any, error) {
		ips, err := d.resolver.LookupIPAddr(context.Background(), host)
		if err != nil {
			return nil, err
		}
		addrs := make([]dnsAddr, 0, len(ips))
		for _, ip := range ips {
			fam := ipFamily(ip.IP)
			if family != 0 && fam != family {
				continue
			}
			addrs = append(addrs, dnsAddr{Address: ip.IP.String(), Family: fam})
		}
		if all {
			return addrs, nil
		}
		if len(addrs) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return addrs[0], nil
	})
	return nil, nil
}

func (d *dnsBridgeState) resolveIP(args []any, want int) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	d.run(id, func() (any, error) {
		network := "ip4"
		if want == 6 {
			network = "ip6"
		}
		ips, err := d.resolver.LookupIP(context.Background(), network, host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ips))
		for _, ip := range ips {
			out = append(out, ip.String())
		}
		return out, nil
	})
	return nil, nil
}

func (d *dnsBridgeState) resolve4(args []any) (any, error) { return d.resolveIP(args, 4) }
func (d *dnsBridgeState) resolve6(args []any) (any, error) { return d.resolveIP(args, 6) }

// dnsMx mirrors Node's MX record shape.
type dnsMx struct {
	Priority int    `json:"priority"`
	Exchange string `json:"exchange"`
}

func (d *dnsBridgeState) resolveMx(args []any) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	d.run(id, func() (any, error) {
		records, err := d.resolver.LookupMX(context.Background(), host)
		if err != nil {
			return nil, err
		}
		out := make([]dnsMx, 0, len(records))
		for _, r := range records {
			out = append(out, dnsMx{Priority: int(r.Pref), Exchange: trimDot(r.Host)})
		}
		return out, nil
	})
	return nil, nil
}

func (d *dnsBridgeState) resolveTxt(args []any) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	d.run(id, func() (any, error) {
		records, err := d.resolver.LookupTXT(context.Background(), host)
		if err != nil {
			return nil, err
		}
		// Node returns an array of arrays: each record is a list of string
		// chunks. Go joins the chunks, so each record becomes a one-element list.
		out := make([][]string, 0, len(records))
		for _, r := range records {
			out = append(out, []string{r})
		}
		return out, nil
	})
	return nil, nil
}

// dnsSrv mirrors Node's SRV record shape.
type dnsSrv struct {
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
	Port     int    `json:"port"`
	Name     string `json:"name"`
}

func (d *dnsBridgeState) resolveSrv(args []any) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	d.run(id, func() (any, error) {
		_, records, err := d.resolver.LookupSRV(context.Background(), "", "", host)
		if err != nil {
			return nil, err
		}
		out := make([]dnsSrv, 0, len(records))
		for _, r := range records {
			out = append(out, dnsSrv{Priority: int(r.Priority), Weight: int(r.Weight), Port: int(r.Port), Name: trimDot(r.Target)})
		}
		return out, nil
	})
	return nil, nil
}

func (d *dnsBridgeState) resolveNs(args []any) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	d.run(id, func() (any, error) {
		records, err := d.resolver.LookupNS(context.Background(), host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(records))
		for _, r := range records {
			out = append(out, trimDot(r.Host))
		}
		sort.Strings(out)
		return out, nil
	})
	return nil, nil
}

func (d *dnsBridgeState) resolveCname(args []any) (any, error) {
	id := int64(intArg(args, 0))
	host := str(args, 1)
	d.run(id, func() (any, error) {
		cname, err := d.resolver.LookupCNAME(context.Background(), host)
		if err != nil {
			return nil, err
		}
		return []string{trimDot(cname)}, nil
	})
	return nil, nil
}

func (d *dnsBridgeState) reverse(args []any) (any, error) {
	id := int64(intArg(args, 0))
	ip := str(args, 1)
	d.run(id, func() (any, error) {
		names, err := d.resolver.LookupAddr(context.Background(), ip)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(names))
		for _, n := range names {
			out = append(out, trimDot(n))
		}
		return out, nil
	})
	return nil, nil
}

// ipFamily reports 4 for an IPv4 address and 6 for IPv6, matching Node's family
// field.
func ipFamily(ip net.IP) int {
	if ip.To4() != nil {
		return 4
	}
	return 6
}

// trimDot drops the trailing dot Go's resolver leaves on fully qualified names;
// Node's records carry none.
func trimDot(s string) string {
	if n := len(s); n > 0 && s[n-1] == '.' {
		return s[:n-1]
	}
	return s
}

// dnsCode maps a resolver error to the code Node puts on the error object. A
// not-found lookup is ENOTFOUND; everything else falls back to the generic
// EAI_AGAIN-style label so callers can still branch on err.code.
func dnsCode(err error) string {
	var de *net.DNSError
	if e, ok := err.(*net.DNSError); ok {
		de = e
	}
	if de != nil {
		if de.IsNotFound {
			return "ENOTFOUND"
		}
		if de.IsTimeout {
			return "ETIMEOUT"
		}
	}
	return "EAI_AGAIN"
}
