package appkit

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtractFindsTheBinaryOneLevelDown(t *testing.T) {
	for ext, data := range map[string][]byte{
		".tar.gz": tarball(t, map[string]string{"demo-1.0.0-linux-x86_64/demo": "new"}),
		".zip":    zipball(t, map[string]string{"demo-1.0.0-windows-x86_64/demo": "new"}),
	} {
		dir := t.TempDir()
		archive := filepath.Join(dir, "a"+ext)
		os.WriteFile(archive, data, 0o644)
		if err := extract(archive, filepath.Join(dir, "out")); err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		p, ok := find(filepath.Join(dir, "out"), "demo")
		if b, _ := os.ReadFile(p); !ok || string(b) != "new" {
			t.Errorf("%s: found %q (%v)", ext, p, ok)
		}
	}
}

func TestExtractRefusesPathsOutsideTheArchive(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	os.WriteFile(archive, tarball(t, map[string]string{"../escaped": "x"}), 0o644)
	if err := extract(archive, filepath.Join(dir, "out")); err == nil {
		t.Fatal("extract accepted ../escaped")
	}
	if exists(filepath.Join(dir, "escaped")) {
		t.Fatal("../escaped was written")
	}
}

// 用本地 HTTP 服务端当 GitHub，走一遍下载、校验、解压、替换。
func TestApplyReplacesTheExecutable(t *testing.T) {
	exeName := "demo"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	name, ok := assetName("demo", "1.1.0", runtime.GOOS, runtime.GOARCH, false)
	if !ok {
		t.Skip("no release builds for this platform")
	}
	var data []byte
	if filepath.Ext(name) == ".zip" {
		data = zipball(t, map[string]string{"demo-1.1.0/" + exeName: "new"})
	} else {
		data = tarball(t, map[string]string{"demo-1.1.0/" + exeName: "new"})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	sum := sha256.Sum256(data)
	asset := Asset{Name: name, URL: srv.URL, Size: int64(len(data)), Digest: "sha256:" + hex.EncodeToString(sum[:])}

	exe := filepath.Join(t.TempDir(), exeName)
	os.WriteFile(exe, []byte("old"), 0o755)
	inst := Install{Kind: Executable, Path: exe}
	var last int64
	err := Apply(context.Background(), demo, inst, Release{Version: "1.1.0", Assets: []Asset{asset}}, func(done, _ int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" || last != int64(len(data)) {
		t.Fatalf("exe = %q, progress ended at %d of %d", b, last, len(data))
	}

	// 校验和对不上就不动原文件
	os.WriteFile(exe, []byte("old"), 0o755)
	asset.Digest = "sha256:" + hex.EncodeToString(make([]byte, 32))
	if err := Apply(context.Background(), demo, inst, Release{Version: "1.1.0", Assets: []Asset{asset}}, nil); err == nil {
		t.Fatal("Apply accepted a wrong SHA-256")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatalf("exe changed to %q after a failed check", b)
	}
}

// .app 整个换掉：旧 bundle 里有、新 bundle 里没有的文件不留下。
func TestApplyReplacesTheBundle(t *testing.T) {
	name, ok := assetName("demo", "1.1.0", runtime.GOOS, runtime.GOARCH, true)
	if !ok {
		t.Skip("no release builds for this platform")
	}
	files := map[string]string{"Demo.app/Contents/MacOS/demo": "new", "README.md": "readme"}
	var data []byte
	if filepath.Ext(name) == ".zip" {
		data = zipball(t, files)
	} else {
		data = tarball(t, files)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	sum := sha256.Sum256(data)
	asset := Asset{Name: name, URL: srv.URL, Size: int64(len(data)), Digest: "sha256:" + hex.EncodeToString(sum[:])}

	dir := t.TempDir()
	app := filepath.Join(dir, "Demo.app")
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	os.MkdirAll(filepath.Join(app, "Contents", "Helpers"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "MacOS", "demo"), []byte("old"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "Helpers", "gone"), []byte("old"), 0o755)

	inst := Install{Kind: Bundle, Path: app}
	if err := Apply(context.Background(), demo, inst, Release{Version: "1.1.0", Assets: []Asset{asset}}, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(app, "Contents", "MacOS", "demo")); string(b) != "new" {
		t.Errorf("Contents/MacOS/demo = %q", b)
	}
	if exists(filepath.Join(app, "Contents", "Helpers", "gone")) {
		t.Error("a file only the old bundle had is still there")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left beside the bundle: %v", entries)
	}
}

func TestPendingUpdateKeepsTheCheckDue(t *testing.T) {
	a := &AutoCheck{state: autoState{Enabled: true}}
	if !a.Due(demo) {
		t.Fatal("never checked: due")
	}
	a.Record(nil)
	if a.Due(demo) {
		t.Fatal("just checked, nothing new: not due")
	}
	a.Record(&Release{Version: "1.1.0"})
	if !a.Due(demo) || a.Pending(demo) != "1.1.0" {
		t.Fatal("newer release pending: due")
	}
	if a.Pending(App{Name: "demo", Version: "1.1.0"}) != "" {
		t.Fatal("pending version already installed")
	}
	a.SetEnabled(false)
	if a.Due(demo) {
		t.Fatal("disabled: not due")
	}
}
