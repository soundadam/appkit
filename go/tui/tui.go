// Package tui 是 bubbletea v1 的更新提示：嵌进应用自己的 model，
// 有新版时 View 给出一行提示，按键后在后台下载替换，显示进度，装好后提示重启。
// 用 bubbletea v2 的应用用 tuiv2，用法相同。
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	appkit "github.com/soundadam/appkit/go"
	"github.com/soundadam/appkit/go/internal/tuicore"
)

// Updater 嵌在应用的 model 里：Init、Update 里转给它，View 里把它的一行放在合适的位置。
type Updater struct {
	// Key 是开始更新的按键，默认 "u"；设成 "" 时由应用自己调 Start。
	Key string

	core tuicore.Core
}

// New 在构造应用的 model 时调用。
func New(app appkit.App) Updater {
	return Updater{Key: "u", core: tuicore.New(app)}
}

func cmd(c tuicore.Cmd) tea.Cmd {
	if c == nil {
		return nil
	}
	return func() tea.Msg { return c() }
}

// Init 到了该自动检查的时候在后台查一次。
func (u Updater) Init() tea.Cmd { return cmd(u.core.Init()) }

// Check 立即检查一次，比如应用里的「检查更新」。
func (u Updater) Check() (Updater, tea.Cmd) {
	var c tuicore.Cmd
	u.core, c = u.core.Check()
	return u, cmd(c)
}

// Available 表示查到了新版、还没开始装。
func (u Updater) Available() bool { return u.core.Available() }

// Updating 表示正在下载替换（或交给 brew 升级），还没结束。
func (u Updater) Updating() bool { return u.core.Updating() }

// Start 开始更新到查到的新版。
func (u Updater) Start() (Updater, tea.Cmd) {
	var c tuicore.Cmd
	u.core, c = u.core.Start()
	return u, cmd(c)
}

// Update 处理自己的消息与按键 Key；别的消息原样忽略。
func (u Updater) Update(msg tea.Msg) (Updater, tea.Cmd) {
	var c tuicore.Cmd
	u.core, c = u.core.Update(msg, pressed(msg, u.Key))
	return u, cmd(c)
}

func pressed(msg tea.Msg, key string) bool {
	k, ok := msg.(tea.KeyMsg)
	return ok && key != "" && k.String() == key
}

// View 是一行提示；没什么可说的时候是 ""。
func (u Updater) View() string { return u.core.View(u.Key) }
