package appkit

import (
	"os"
	"path/filepath"
	"strings"
)

// Kind 是正在运行的这份程序怎么装上的，决定更新走哪条路。
type Kind int

const (
	// Executable 是下载来的单个可执行文件，原地替换。
	Executable Kind = iota
	// Bundle 是 macOS 上从 Releases 解压出来的 .app，整个 bundle 原地替换。
	Bundle
	// Homebrew 是 Cask 装的。升级交给 brew，自己不动文件，否则 brew 的记录就错了。
	Homebrew
	// Dev 是开发构建，不更新。
	Dev
)

// Install 是正在运行的这份程序。
type Install struct {
	Kind Kind
	// Path 是解开符号链接后的可执行文件；Bundle 时是它所在的 .app。
	Path string
	// Brew 是 Homebrew 安装时的 brew 可执行文件。
	Brew string
}

// Detect 判断正在运行的这份程序是怎么装上的。
func Detect(app App) Install {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return Install{Kind: Dev}
	}
	return classify(app, exe, homebrewPrefixes())
}

func homebrewPrefixes() []string {
	var prefixes []string
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		prefixes = append(prefixes, p)
	}
	prefixes = append(prefixes, "/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew")
	if home, err := os.UserHomeDir(); err == nil {
		prefixes = append(prefixes, filepath.Join(home, ".linuxbrew"))
	}
	return prefixes
}

// bundleOf 是可执行文件所在的最外层 .app（"X.app/Contents/…/x"）。
// Contents 下哪个目录都算：MacOS 里的主程序，Helpers 里随 .app 发的命令行也是。
func bundleOf(exe string) (string, bool) {
	sep := string(filepath.Separator)
	parts := strings.Split(exe, sep)
	for i := 0; i+1 < len(parts); i++ {
		if strings.HasSuffix(parts[i], ".app") && parts[i+1] == "Contents" {
			return strings.Join(parts[:i+1], sep), true
		}
	}
	return "", false
}

func classify(app App, exe string, prefixes []string) Install {
	if app.Dev() {
		return Install{Kind: Dev, Path: exe}
	}
	bundle, inBundle := bundleOf(exe)
	for _, prefix := range prefixes {
		caskroom := filepath.Join(prefix, "Caskroom", app.Name)
		brew := filepath.Join(prefix, "bin", "brew")
		// Linux 上 Cask 的 binary 是指进 Caskroom 的符号链接
		if strings.HasPrefix(exe, caskroom+string(filepath.Separator)) {
			return Install{Kind: Homebrew, Path: exe, Brew: brew}
		}
		// macOS 上 Cask 把 .app 移到 Applications，Caskroom 里只留记录
		if inBundle && filepath.Base(filepath.Dir(bundle)) == "Applications" {
			if fi, err := os.Stat(caskroom); err == nil && fi.IsDir() {
				return Install{Kind: Homebrew, Path: exe, Brew: brew}
			}
		}
	}
	if inBundle {
		return Install{Kind: Bundle, Path: bundle}
	}
	return Install{Kind: Executable, Path: exe}
}
