package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func request(t *testing.T, a *app, method, path, body, token, origin, host string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:12345"+path, strings.NewReader(body))
	r.Host = host
	r.Header.Set("X-EasyPackage-Token", token)
	r.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	return w
}
func TestLocalSecurity(t *testing.T) {
	a := &app{token: "secret", host: "127.0.0.1:12345", stop: make(chan struct{}), state: job{Status: "idle"}}
	for _, tc := range []struct {
		name, token, origin, host string
		status                    int
	}{{"valid", "secret", "", a.host, 200}, {"no token", "", "", a.host, 401}, {"cross origin", "secret", "https://example.com", a.host, 403}, {"DNS rebinding", "secret", "", "example.com", 403}} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(t, a, "GET", "/api/state", "", tc.token, tc.origin, tc.host)
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
	w := request(t, a, "GET", "/", "", "", "", a.host)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "好项目") {
		t.Fatal("embedded UI unavailable")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing CSP")
	}
	w = request(t, a, "GET", "/api/cancel", "", "secret", "", a.host)
	if w.Code != http.StatusNotFound {
		t.Fatal("mutation accepted by GET")
	}
}
func TestAPIPackAndBusyConflict(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "project")
	os.Mkdir(source, 0755)
	os.WriteFile(filepath.Join(source, "hello.txt"), []byte("hello"), 0644)
	a := &app{token: "secret", host: "127.0.0.1:12345", stop: make(chan struct{}), state: job{Status: "idle"}}
	data := map[string]any{"source": source, "output": filepath.Join(dir, "out.zip"), "filterClass": true, "action": "pack"}
	b, _ := json.Marshal(data)
	w := request(t, a, "POST", "/api/start", string(b), a.token, "", a.host)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		a.mu.Lock()
		state := a.state
		a.mu.Unlock()
		if state.Status != "running" {
			if state.Status != "done" {
				t.Fatalf("job: %+v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, e := os.Stat(data["output"].(string)); e != nil {
		t.Fatal(e)
	}
	w = request(t, a, "POST", "/api/start", string(b), a.token, "", a.host)
	if w.Code != 409 {
		t.Fatalf("overwrite should request confirmation: %d", w.Code)
	}
	a.mu.Lock()
	a.state.Status = "running"
	_, a.cancel = context.WithCancel(context.Background())
	a.mu.Unlock()
	data["action"] = "preview"
	b, _ = json.Marshal(data)
	w = request(t, a, "POST", "/api/start", string(b), a.token, "", a.host)
	if w.Code != 423 {
		t.Fatal("parallel job allowed")
	}
	w = request(t, a, "POST", "/api/cancel", "{}", a.token, "", a.host)
	if w.Code != 200 {
		t.Fatal("cancel endpoint failed")
	}
	w = request(t, a, "POST", "/api/start", `{"action":"pack","unexpected":true}`, a.token, "", a.host)
	if w.Code != 400 {
		t.Fatal("unknown fields accepted")
	}
}
