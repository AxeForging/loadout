package integration_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallScript_InstallsVerifiedRelease(t *testing.T) {
	if runtime.GOOS == "windows" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("unsupported installer platform")
	}
	tmp := t.TempDir()
	payload := filepath.Join(tmp, "payload")
	writeExecutable(t, filepath.Join(payload, "loadout"), "#!/bin/sh\nprintf 'installed loadout\\n'\n")
	asset := "loadout-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	archive := filepath.Join(tmp, asset)
	if out, err := exec.Command("tar", "-czf", archive, "-C", payload, "loadout").CombinedOutput(); err != nil {
		t.Fatalf("archive: %v %s", err, out)
	}
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sums := filepath.Join(tmp, "checksums.txt")
	if err := os.WriteFile(sums, []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(b), asset)), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "bin")
	fake := `#!/bin/sh
set -eu
while [ "$#" -gt 0 ]; do case "$1" in -o) out="$2"; shift 2;; -*) shift;; *) url="$1"; shift;; esac; done
printf '%s\n' "$url" >> "$CURL_LOG"; case "$url" in */checksums.txt) cp "$SUMS" "$out";; *) cp "$ARCHIVE" "$out";; esac
`
	writeExecutable(t, filepath.Join(bin, "curl"), fake)
	dest, log := filepath.Join(tmp, "install"), filepath.Join(tmp, "curl.log")
	cmd := exec.Command("sh", filepath.Join("..", "install.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "LOADOUT_VERSION=v1.4.0", "LOADOUT_INSTALL_DIR="+dest, "ARCHIVE="+archive, "SUMS="+sums, "CURL_LOG="+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install: %v %s", err, out)
	}
	out, err := exec.Command(filepath.Join(dest, "loadout")).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "installed loadout" {
		t.Fatalf("binary: %v %q", err, out)
	}
	logged, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "/releases/download/v1.4.0/"+asset) {
		t.Fatalf("wrong URL: %s", logged)
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create executable directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}
