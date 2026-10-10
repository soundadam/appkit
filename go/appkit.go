// Package appkit 是 soundadam 用 Go 写的应用的公共底座：版本、安装来源、检查更新与自更新。
// 产物命名、Cask、命令行与更新的约定见仓库根目录的 README。
//
// 子包 cli 给 cobra 的根命令补上共用的命令，子包 tui 是 bubbletea 的更新提示。
package appkit

import (
	"strings"

	"golang.org/x/mod/semver"
)

// App 是一个应用的身份。
type App struct {
	// Name 是可执行文件名，也是 Cask 名、产物文件名前缀。
	Name string
	// Version 在构建时用 -ldflags "-X main.version=<ver>" 注入；
	// 不是 semver（通常是 "dev"）就是开发构建。
	Version string
	// Repo 是 GitHub 上的 "owner/repo"。
	Repo string
}

// Dev 表示开发构建，不更新。
func (a App) Dev() bool {
	return !semver.IsValid(canonical(a.Version))
}

// Cask 是仓库主的 tap 里的同名 Cask："<owner>/tap/<name>"。
func (a App) Cask() string {
	owner, _, _ := strings.Cut(a.Repo, "/")
	return owner + "/tap/" + a.Name
}

// canonical 把 "1.2.3" 或 "v1.2.3" 变成 x/mod/semver 要的 "v1.2.3"。
func canonical(v string) string {
	return "v" + strings.TrimPrefix(v, "v")
}

// newer 表示版本 a 比 b 新；任一边不是 semver 都不算新。
func newer(a, b string) bool {
	a, b = canonical(a), canonical(b)
	return semver.IsValid(a) && semver.IsValid(b) && semver.Compare(a, b) > 0
}
