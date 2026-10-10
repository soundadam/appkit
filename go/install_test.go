package appkit

import "testing"

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
		// 名字开头相同的另一个应用的 Caskroom 不算
		{demo, prefix + "/Caskroom/demo-extra/1.0.0/demo", Install{Kind: Executable, Path: prefix + "/Caskroom/demo-extra/1.0.0/demo"}},
		{demo, "/home/u/bin/demo", Install{Kind: Executable, Path: "/home/u/bin/demo"}},
		{App{Name: "demo", Version: "dev", Repo: "owner/demo"}, "/home/u/bin/demo", Install{Kind: Dev, Path: "/home/u/bin/demo"}},
	}
	for _, c := range cases {
		if got := classify(c.app, c.exe, []string{prefix}); got != c.want {
			t.Errorf("classify(%q) = %+v, want %+v", c.exe, got, c.want)
		}
	}
}

func TestNewer(t *testing.T) {
	if !newer("0.2.0", "v0.1.9") || newer("0.1.0", "0.1.0") || newer("1.0.0", "dev") {
		t.Fatal("newer 按 semver 比较，不是 semver 的不算新")
	}
}
