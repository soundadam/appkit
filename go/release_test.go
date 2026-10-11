package appkit

import "testing"

func TestAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch string
		bundle       bool
		want         string
	}{
		{"linux", "arm64", false, "demo-1.2.3-linux-aarch64.tar.gz"},
		{"darwin", "amd64", false, "demo-1.2.3-macos-x86_64.tar.gz"},
		{"darwin", "arm64", true, "demo-1.2.3-macos-universal.zip"},
		{"windows", "amd64", false, "demo-1.2.3-windows-x86_64.zip"},
	}
	for _, c := range cases {
		if got, ok := assetName("demo", "1.2.3", c.goos, c.goarch, c.bundle); !ok || got != c.want {
			t.Errorf("assetName(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
	if _, ok := assetName("demo", "1.2.3", "freebsd", "amd64", false); ok {
		t.Error("freebsd has no release builds")
	}
}
