package appkit

import (
	"os"
	"path/filepath"
	"testing"
)

var demo = App{Name: "demo", Version: "1.0.0", Repo: "owner/demo"}

func TestClassify(t *testing.T) {
	prefix := "/home/linuxbrew/.linuxbrew"
	cases := []struct {
		app  App
		exe  string
		want Install
	}{
		{demo, prefix + "/Caskroom/demo/1.0.0/demo-1.0.0-linux-x86_64/demo",
			Install{Kind: Homebrew, Path: prefix + "/Caskroom/demo/1.0.0/demo-1.0.0-linux-x86_64/demo", Brew: prefix + "/bin/brew"}},
		// Formula：opt 与 bin 下的链接都解到 Cellar 里带版本号的目录
		{demo, prefix + "/Cellar/demo/1.0.0/bin/demo",
			Install{Kind: Homebrew, Path: prefix + "/Cellar/demo/1.0.0/bin/demo", Brew: prefix + "/bin/brew", Formula: true}},
		// 名字开头相同的另一个应用的 Caskroom、Cellar 不算
		{demo, prefix + "/Cellar/demo-extra/1.0.0/bin/demo", Install{Kind: Executable, Path: prefix + "/Cellar/demo-extra/1.0.0/bin/demo"}},
		{demo, prefix + "/Caskroom/demo-extra/1.0.0/demo", Install{Kind: Executable, Path: prefix + "/Caskroom/demo-extra/1.0.0/demo"}},
		{demo, "/home/u/bin/demo", Install{Kind: Executable, Path: "/home/u/bin/demo"}},
		{App{Name: "demo", Version: "dev", Repo: "owner/demo"}, "/home/u/bin/demo", Install{Kind: Dev, Path: "/home/u/bin/demo"}},
	}
	for _, c := range cases {
		// 路径照本机的分隔符写，Windows 上也测同一条规则
		c.exe, c.want.Path, c.want.Brew = filepath.FromSlash(c.exe), filepath.FromSlash(c.want.Path), filepath.FromSlash(c.want.Brew)
		if got := classify(c.app, c.exe, []string{filepath.FromSlash(prefix)}); got != c.want {
			t.Errorf("classify(%q) = %+v, want %+v", c.exe, got, c.want)
		}
	}
}

func TestNewer(t *testing.T) {
	if !newer("0.2.0", "v0.1.9") || newer("0.1.0", "0.1.0") || newer("1.0.0", "dev") {
		t.Fatal("newer 按 semver 比较，不是 semver 的不算新")
	}
}

func TestClassifyBundle(t *testing.T) {
	prefix := t.TempDir()
	cases := []struct {
		exe  string
		want Install
	}{
		{"/tmp/x/Demo.app/Contents/MacOS/demo", Install{Kind: Bundle, Path: "/tmp/x/Demo.app"}},
		// 随 .app 发的命令行
		{"/tmp/x/Demo.app/Contents/Helpers/demo", Install{Kind: Bundle, Path: "/tmp/x/Demo.app"}},
		// 还没有 Caskroom 记录，Applications 里的也是自己解压的
		{"/Applications/Demo.app/Contents/MacOS/demo", Install{Kind: Bundle, Path: "/Applications/Demo.app"}},
		// 名字里有 .app、后面不是 Contents 的目录不是 bundle
		{"/home/u/my.app/bin/demo", Install{Kind: Executable, Path: "/home/u/my.app/bin/demo"}},
	}
	for _, c := range cases {
		c.exe, c.want.Path = filepath.FromSlash(c.exe), filepath.FromSlash(c.want.Path)
		if got := classify(demo, c.exe, []string{prefix}); got != c.want {
			t.Errorf("classify(%q) = %+v, want %+v", c.exe, got, c.want)
		}
	}

	os.MkdirAll(filepath.Join(prefix, "Caskroom", "demo"), 0o755)
	exe := filepath.FromSlash("/Applications/Demo.app/Contents/Helpers/demo")
	want := Install{Kind: Homebrew, Path: exe, Brew: filepath.Join(prefix, "bin", "brew")}
	if got := classify(demo, exe, []string{prefix}); got != want {
		t.Errorf("with a Caskroom record: %+v", got)
	}
	// 解压在别处的另一份不算 brew 的
	exe = filepath.FromSlash("/tmp/x/Demo.app/Contents/MacOS/demo")
	if got := classify(demo, exe, []string{prefix}); got.Kind != Bundle {
		t.Errorf("a copy outside Applications: %+v", got)
	}
}
