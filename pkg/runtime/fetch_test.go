package runtime

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

func TestHeadersClass(t *testing.T) {
	script := `
		const h = new Headers({ "Content-Type": "text/plain" });
		h.append("X-Tag", "a");
		h.append("X-Tag", "b");
		console.log(h.get("content-type"));
		console.log(h.get("x-tag"));
		console.log(h.has("x-tag") + " " + h.has("nope"));
		h.set("x-tag", "one");
		console.log(h.get("x-tag"));
		h.delete("x-tag");
		console.log(String(h.get("x-tag")));
		const from = new Headers([["a", "1"], ["b", "2"]]);
		console.log(Array.from(from.keys()).join(","));
	`
	out := strings.TrimSpace(runToEnd(t, script))
	want := "text/plain\na, b\ntrue false\none\nnull\na,b"
	if out != want {
		t.Errorf("headers = %q, want %q", out, want)
	}
}

func TestResponseBodyHelpers(t *testing.T) {
	script := `
		const r = new Response(JSON.stringify({ hi: 1 }), { status: 201, headers: { "Content-Type": "application/json" } });
		console.log(r.status + " " + r.ok + " " + r.headers.get("content-type"));
		r.json().then((data) => { console.log(data.hi); });
		const t = new Response("plain body");
		t.text().then((s) => { console.log(s); });
		const j = Response.json({ ok: true });
		console.log(j.headers.get("content-type"));
	`
	out := strings.TrimSpace(runToEnd(t, script))
	// json() chains text().then(json), one microtask deeper than the bare text(),
	// so "plain body" settles before "1".
	want := "201 true application/json\napplication/json\nplain body\n1"
	if out != want {
		t.Errorf("response helpers = %q, want %q", out, want)
	}
}

func TestFetchGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Origin", "fetch-test")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"path":"`+r.URL.Path+`","method":"`+r.Method+`"}`)
	}))
	defer srv.Close()

	script := `
		fetch(` + jsQuote(srv.URL+"/data") + `).then((res) => {
			console.log(res.status + " " + res.ok + " " + res.headers.get("x-origin"));
			return res.json();
		}).then((body) => {
			console.log(body.path + " " + body.method);
		}).catch((err) => { console.log("ERR " + err.message); });
	`
	out := strings.TrimSpace(runToEnd(t, script))
	want := "200 true fetch-test\n/data GET"
	if out != want {
		t.Errorf("fetch get = %q, want %q", out, want)
	}
}

func TestFetchPostJSON(t *testing.T) {
	var got struct {
		CT   string
		Body string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got.CT = r.Header.Get("Content-Type")
		got.Body = string(body)
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	script := `
		fetch(` + jsQuote(srv.URL+"/submit") + `, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ name: "duc" }),
		}).then((res) => res.text()).then((text) => {
			console.log(text);
		}).catch((err) => { console.log("ERR " + err.message); });
	`
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "ok" {
		t.Errorf("fetch post = %q, want ok", out)
	}
	if got.CT != "application/json" {
		t.Errorf("content-type = %q, want application/json", got.CT)
	}
	if got.Body != `{"name":"duc"}` {
		t.Errorf("body = %q, want %q", got.Body, `{"name":"duc"}`)
	}
}

func TestFetchStringBodyDefaultsContentType(t *testing.T) {
	ct := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct <- r.Header.Get("Content-Type")
		w.WriteHeader(204)
	}))
	defer srv.Close()

	script := `
		fetch(` + jsQuote(srv.URL) + `, { method: "POST", body: "hello" })
			.then((res) => { console.log(res.status); })
			.catch((err) => { console.log("ERR " + err.message); });
	`
	out := strings.TrimSpace(runToEnd(t, script))
	if out != "204" {
		t.Errorf("status = %q, want 204", out)
	}
	if got := <-ct; !strings.HasPrefix(got, "text/plain") {
		t.Errorf("default content-type = %q, want text/plain...", got)
	}
}

func TestRequestObject(t *testing.T) {
	script := `
		const req = new Request("https://example.com/api", {
			method: "post",
			headers: { "X-Key": "v" },
			body: JSON.stringify({ a: 1 }),
		});
		console.log(req.method + " " + req.url);
		console.log(req.headers.get("x-key"));
		console.log(req.headers.get("content-type"));
		req.json().then((d) => { console.log(d.a); });
	`
	out := strings.TrimSpace(runToEnd(t, script))
	want := "POST https://example.com/api\nv\ntext/plain;charset=UTF-8\n1"
	if out != want {
		t.Errorf("request object = %q, want %q", out, want)
	}
}

// jsQuote renders a Go string as a JSON string literal so it drops safely into a
// JavaScript source template.
func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
