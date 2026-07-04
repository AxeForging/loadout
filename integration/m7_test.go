//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestServe_SmokeServesSPAAndAPI boots the real binary's web server and hits
// the SPA shell plus the API, including the two-phase sync gate over HTTP.
func TestServe_SmokeServesSPAAndAPI(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cmd := exec.Command(binary(t), "serve", "--addr", addr)
	cmd.Env = append(cmd.Env,
		"HOME="+e.home,
		"XDG_CONFIG_HOME="+e.home+"/.config",
		"XDG_DATA_HOME="+e.home+"/.local/share",
		"PATH=/usr/bin:/bin",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	base := "http://" + addr
	waitReady(t, base+"/api/overview")

	// SPA shell.
	body := get(t, base+"/")
	if !strings.Contains(body, "<html") {
		t.Fatalf("index not served:\n%.200s", body)
	}
	// Client-route fallback.
	if fallback := get(t, base+"/agents"); !strings.Contains(fallback, "<html") {
		t.Fatal("client route did not fall back to index.html")
	}

	// API overview.
	var overview struct {
		Pending int `json:"pending"`
	}
	if err := json.Unmarshal([]byte(get(t, base+"/api/overview")), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Pending != 1 {
		t.Fatalf("pending = %d, want 1", overview.Pending)
	}

	// Two-phase sync over HTTP.
	var plan struct {
		Token   string `json:"token"`
		Changes []any  `json:"changes"`
	}
	if err := json.Unmarshal([]byte(post(t, base+"/api/sync/plan", "{}")), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Token == "" || len(plan.Changes) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	applied := post(t, base+"/api/sync/apply", fmt.Sprintf(`{"token":%q}`, plan.Token))
	if !strings.Contains(applied, `"applied":1`) {
		t.Fatalf("apply = %s", applied)
	}
}

func waitReady(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(url)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
}

func get(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func post(t *testing.T, url, body string) string {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
