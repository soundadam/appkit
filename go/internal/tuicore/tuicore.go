// Package tuicore 是更新提示的状态机，不依赖 bubbletea；
// tui（bubbletea v1）与 tuiv2（v2）只是把它的消息和命令换成各自的类型。
package tuicore

import (
	"context"
	"fmt"

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

// Cmd 对应 bubbletea 的 tea.Cmd；nil 表示没有要做的。
type Cmd func() any

// Core 是一个更新提示。值语义，与 bubbletea 的 model 一样每次 Update 返回新的一份。
type Core struct {
	app      appkit.App
	inst     appkit.Install
	auto     *appkit.AutoCheck
	status   status
	release  *appkit.Release
	progress float64 // 0..1；交给 brew 时是 -1
	err      error
	events   chan any
}

type checkedMsg struct {
	found *appkit.Release
	err   error
}

type progressMsg float64

type doneMsg struct{ err error }

func New(app appkit.App) Core {
	return Core{app: app, inst: appkit.Detect(app), auto: appkit.LoadAutoCheck(app)}
}

// Init 到了该自动检查的时候在后台查一次。
func (c Core) Init() Cmd {
	if !c.auto.Due(c.app) {
		return nil
	}
	return c.checkCmd()
}

func (c Core) checkCmd() Cmd {
	app := c.app
	return func() any {
		found, err := appkit.Check(context.Background(), app)
		return checkedMsg{found, err}
	}
}

func (c Core) Check() (Core, Cmd) {
	if c.status == updating || c.status == ready {
		return c, nil
	}
	c.status = checking
	return c, c.checkCmd()
}

func (c Core) Available() bool { return c.status == available }

func (c Core) Start() (Core, Cmd) {
	if c.status != available || c.inst.Kind == appkit.Dev {
		return c, nil
	}
	c.status, c.progress = updating, 0
	if c.inst.Kind == appkit.Homebrew {
		c.progress = -1
	}
	c.events = make(chan any, 16)
	go func(app appkit.App, inst appkit.Install, rel appkit.Release, events chan<- any) {
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
	}(c.app, c.inst, *c.release, c.events)
	return c, c.next()
}

func (c Core) next() Cmd {
	events := c.events
	return func() any { return <-events }
}

// Update 处理自己的消息；pressed 表示 msg 是开始更新的那个键。别的消息原样忽略。
func (c Core) Update(msg any, pressed bool) (Core, Cmd) {
	if pressed {
		return c.Start()
	}
	switch msg := msg.(type) {
	case checkedMsg:
		if msg.err != nil {
			// 自动检查失败不打扰；手动检查才显示
			if c.status == checking {
				c.status, c.err = failed, msg.err
			}
			return c, nil
		}
		c.auto.Record(msg.found)
		c.release = msg.found
		c.status = idle
		if msg.found != nil {
			c.status = available
		}
	case progressMsg:
		c.progress = float64(msg)
		return c, c.next()
	case doneMsg:
		c.status, c.err = ready, msg.err
		if msg.err != nil {
			c.status = failed
		}
	}
	return c, nil
}

// View 是一行提示；没什么可说的时候是 ""。key 是开始更新的按键，"" 表示应用自己调 Start。
func (c Core) View(key string) string {
	switch c.status {
	case available:
		s := fmt.Sprintf("⬆ %s %s is available", c.app.Name, c.release.Version)
		if c.inst.Kind == appkit.Dev {
			return s + " (development build: updates are disabled)"
		}
		if key != "" {
			s += fmt.Sprintf(" — press %s to update", key)
		}
		return s
	case updating:
		if c.progress < 0 {
			return fmt.Sprintf("Updating to %s with Homebrew…", c.release.Version)
		}
		return fmt.Sprintf("Updating to %s… %3.0f%%", c.release.Version, c.progress*100)
	case ready:
		return fmt.Sprintf("⟳ Updated to %s — restart %s to use it", c.release.Version, c.app.Name)
	case failed:
		return "Update failed: " + c.err.Error()
	}
	return ""
}
