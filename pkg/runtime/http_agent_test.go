package runtime

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestHTTPAgentKeepAliveReuse checks that two sequential requests through a
// keep-alive Agent reuse one TCP connection, while an Agent with keepAlive off
// opens a fresh one each time. The server reports the client's remote address so
// the test can tell connections apart.
func TestHTTPAgentKeepAliveReuse(t *testing.T) {
	var mu sync.Mutex
	remotes := map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		remotes[r.URL.Path] = append(remotes[r.URL.Path], r.RemoteAddr)
		mu.Unlock()
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	// keep chains two requests through one keep-alive agent; noka does the same
	// through an agent with keepAlive disabled. Each request runs after the prior
	// one's body is drained so a reused connection is idle and available.
	script := fmt.Sprintf(`
		const http = require("http");
		const base = %s;

		function once(agent, path) {
			return new Promise((resolve, reject) => {
				const req = http.request(base + path, { agent: agent }, (res) => {
					let body = "";
					res.on("data", (c) => (body += c));
					res.on("end", () => resolve(body));
				});
				req.on("error", reject);
				req.end();
			});
		}

		async function main() {
			const keep = new http.Agent({ keepAlive: true });
			await once(keep, "/keep");
			await once(keep, "/keep");
			await once(false, "/noka");
			await once(false, "/noka");
			console.log("done");
		}
		main();
	`, jsQuote(srv.URL))

	out := runToEnd(t, script)
	if !strings.Contains(out, "done") {
		t.Fatalf("agent script did not finish: %q", out)
	}

	mu.Lock()
	defer mu.Unlock()
	keep := remotes["/keep"]
	noka := remotes["/noka"]
	if len(keep) != 2 || len(noka) != 2 {
		t.Fatalf("expected two requests each, got keep=%v noka=%v", keep, noka)
	}
	if keep[0] != keep[1] {
		t.Errorf("keep-alive agent did not reuse the connection: %v", keep)
	}
	if noka[0] == noka[1] {
		t.Errorf("keepAlive:false agent reused a connection: %v", noka)
	}
}
