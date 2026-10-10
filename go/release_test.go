package appkit

import "testing"

func TestAssetName(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"linux", "arm64", "demo-1.2.3-linux-aarch64.tar.gz"},
		{"darwin", "amd64", "demo-1.2.3-macos-x86_64.tar.gz"},
		{"windows", "amd64", "demo-1.2.3-windows-x86_64.zip"},
	}
	for _, c := range cases {
		if got, ok := assetName("demo", "1.2.3", c.goos, c.goarch); !ok || got != c.want {
			t.Errorf("assetName(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
	if _, ok := assetName("demo", "1.2.3", "freebsd", "amd64"); ok {
		t.Error("freebsd has no release builds")
	}
}
