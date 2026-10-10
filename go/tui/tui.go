// Package tui 是 bubbletea 的更新提示：嵌进应用自己的 model，
// 有新版时 View 给出一行提示，按键后在后台下载替换，显示进度，装好后提示重启。
package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appkit "github.com/soundadam/appkit/go"
)

type status int

const (
	idle status = iota
	checking
	available
	updating
	ready
	failed
)

// Updater 嵌在应用的 model 里：Init、Update 里转给它，View 里把它的一行放在合适的位置。
type Updater struct {
	// Key 是开始更新的按键，默认 "u"；设成 "" 时由应用自己调 Start。
	Key string

	app      appkit.App
	inst     appkit.Install
	auto     *appkit.AutoCheck
	status   status
	release  *appkit.Release
	progress float64 // 0..1；交给 brew 时是 -1
	err      error
	events   chan tea.Msg
}

type checkedMsg struct {
	found *appkit.Release
	err   error
}

type progressMsg float64

type doneMsg struct{ err error }

// New 在构造应用的 model 时调用。
func New(app appkit.App) Updater {
	return Updater{Key: "u", app: app, inst: appkit.Detect(app), auto: appkit.LoadAutoCheck(app)}
}

// Init 到了该自动检查的时候在后台查一次。
func (u Updater) Init() tea.Cmd {
	if !u.auto.Due(u.app) {
		return nil
	}
	return u.checkCmd()
}

func (u Updater) checkCmd() tea.Cmd {
	app := u.app
	return func() tea.Msg {
		found, err := appkit.Check(context.Background(), app)
		return checkedMsg{found, err}
	}
}

// Check 立即检查一次，比如应用里的「检查更新」。
func (u Updater) Check() (Updater, tea.Cmd) {
	if u.status == updating || u.status == ready {
		return u, nil
	}
	u.status = checking
	return u, u.checkCmd()
}

// Available 表示查到了新版、还没开始装。
func (u Updater) Available() bool { return u.status == available }

// Start 开始更新到查到的新版。
func (u Updater) Start() (Updater, tea.Cmd) {
	if u.status != available || u.inst.Kind == appkit.Dev {
		return u, nil
	}
	u.status, u.progress = updating, 0
	if u.inst.Kind == appkit.Homebrew {
		u.progress = -1
	}
	u.events = make(chan tea.Msg, 16)
	go func(app appkit.App, inst appkit.Install, rel appkit.Release, events chan<- tea.Msg) {
		var err error
		if inst.Kind == appkit.Homebrew {
			err = appkit.BrewUpgrade(context.Background(), app, inst.Brew)
		} else {
			err = appkit.Apply(context.Background(), app, inst, rel, func(done, total int64) {
				if total > 0 {
					select {
					case events <- progressMsg(float64(done) / float64(total)):
					default: // 界面没跟上就跳过这一格
					}
				}
			})
		}
		events <- doneMsg{err}
	}(u.app, u.inst, *u.release, u.events)
	return u, u.next()
}

func (u Updater) next() tea.Cmd {
	events := u.events
	return func() tea.Msg { return <-events }
}

// Update 处理自己的消息与按键 Key；别的消息原样忽略。
func (u Updater) Update(msg tea.Msg) (Updater, tea.Cmd) {
	switch msg := msg.(type) {
	case checkedMsg:
		if msg.err != nil {
			// 自动检查失败不打扰；手动检查才显示
			if u.status == checking {
				u.status, u.err = failed, msg.err
			}
			return u, nil
		}
		u.auto.Record(msg.found)
		u.release = msg.found
		u.status = idle
		if msg.found != nil {
			u.status = available
		}
	case progressMsg:
		u.progress = float64(msg)
		return u, u.next()
	case doneMsg:
		u.status, u.err = ready, msg.err
		if msg.err != nil {
			u.status = failed
		}
	case tea.KeyMsg:
		if u.Key != "" && msg.String() == u.Key {
			return u.Start()
		}
	}
	return u, nil
}

// View 是一行提示；没什么可说的时候是 ""。
func (u Updater) View() string {
	switch u.status {
	case available:
		s := fmt.Sprintf("⬆ %s %s is available", u.app.Name, u.release.Version)
		if u.inst.Kind == appkit.Dev {
			return s + " (development build: updates are disabled)"
		}
		if u.Key != "" {
			s += fmt.Sprintf(" — press %s to update", u.Key)
		}
		return s
	case updating:
		if u.progress < 0 {
			return fmt.Sprintf("Updating to %s with Homebrew…", u.release.Version)
		}
		return fmt.Sprintf("Updating to %s… %3.0f%%", u.release.Version, u.progress*100)
	case ready:
		return fmt.Sprintf("⟳ Updated to %s — restart %s to use it", u.release.Version, u.app.Name)
	case failed:
		return "Update failed: " + u.err.Error()
	}
	return ""
}
