package appkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// interval 是两次自动检查的最短间隔。GitHub 未登录 API 每个 IP 每小时 60 次，一天一次远低于它。
const interval = 24 * time.Hour

// AutoCheck 是自动检查的开关与节奏，存在 <系统配置目录>/<name>/update.json。
// 存不下（目录不可写）就只在本次运行内生效，不影响程序本身。
type AutoCheck struct {
	path  string
	state autoState
}

type autoState struct {
	Enabled bool `json:"enabled"`
	// LastCheck 是上次检查的 Unix 时间（秒）。
	LastCheck int64 `json:"last_check"`
	// Pending 是上次查到、还没装上的版本；装上之前每次都查，好把提示一直挂着。
	Pending *string `json:"pending"`
}

// LoadAutoCheck 读出 app 的自动检查状态；没有文件就是默认的开着。
func LoadAutoCheck(app App) *AutoCheck {
	a := &AutoCheck{state: autoState{Enabled: true}}
	if dir, err := os.UserConfigDir(); err == nil {
		a.path = filepath.Join(dir, app.Name, "update.json")
		if b, err := os.ReadFile(a.path); err == nil {
			_ = json.Unmarshal(b, &a.state)
		}
	}
	return a
}

func (a *AutoCheck) Enabled() bool { return a.state.Enabled }

func (a *AutoCheck) SetEnabled(on bool) {
	a.state.Enabled = on
	a.save()
}

// Pending 是上次查到、比 app 新、还没装上的版本，没有就是 ""。
func (a *AutoCheck) Pending(app App) string {
	if a.state.Pending != nil && newer(*a.state.Pending, app.Version) {
		return *a.state.Pending
	}
	return ""
}

// Due 表示现在该不该自动去查。开发构建永远不查。
func (a *AutoCheck) Due(app App) bool {
	if !a.state.Enabled || app.Dev() {
		return false
	}
	last := time.Unix(a.state.LastCheck, 0)
	return time.Since(last) >= interval || a.Pending(app) != ""
}

// Record 记下一次检查（自动或手动）的结果；found 为 nil 表示已是最新。
func (a *AutoCheck) Record(found *Release) {
	a.state.LastCheck = time.Now().Unix()
	a.state.Pending = nil
	if found != nil {
		v := found.Version
		a.state.Pending = &v
	}
	a.save()
}

func (a *AutoCheck) save() {
	if a.path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(a.path), 0o755)
	if b, err := json.MarshalIndent(a.state, "", "  "); err == nil {
		_ = os.WriteFile(a.path, b, 0o644)
	}
}
