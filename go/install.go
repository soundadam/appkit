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
	// Homebrew 是 Cask 装的。升级交给 brew，自己不动文件，否则 brew 的记录就错了。
	Homebrew
	// Dev 是开发构建，不更新。
	Dev
)

// Install 是正在运行的这份程序。
type Install struct {
	Kind Kind
	// Path 是解开符号链接后的可执行文件。
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

func classify(app App, exe string, prefixes []string) Install {
	if app.Dev() {
		return Install{Kind: Dev, Path: exe}
	}
	for _, prefix := range prefixes {
		// Cask 的 binary 是指进 Caskroom 的符号链接
		caskroom := filepath.Join(prefix, "Caskroom", app.Name) + string(filepath.Separator)
		if strings.HasPrefix(exe, caskroom) {
			return Install{Kind: Homebrew, Path: exe, Brew: filepath.Join(prefix, "bin", "brew")}
		}
	}
	return Install{Kind: Executable, Path: exe}
}
