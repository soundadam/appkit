package tuicore

import (
	"errors"
	"strings"
	"testing"

	appkit "github.com/soundadam/appkit/go"
)

func core() Core {
	return Core{
		app:  appkit.App{Name: "demo", Version: "1.0.0", Repo: "owner/demo"},
		inst: appkit.Install{Kind: appkit.Executable, Path: "/nonexistent/demo"},
		auto: &appkit.AutoCheck{},
	}
}

func TestQuietUntilSomethingIsFound(t *testing.T) {
	c := core()
	c, _ = c.Update(checkedMsg{found: nil}, false)
	if c.View("u") != "" {
		t.Errorf("up to date: %q", c.View("u"))
	}
	c, _ = c.Update(checkedMsg{err: errors.New("offline")}, false)
	if c.View("u") != "" {
		t.Errorf("a failed automatic check shows %q", c.View("u"))
	}
	c, _ = c.Update(checkedMsg{found: &appkit.Release{Version: "1.1.0"}}, false)
	if !c.Available() || !strings.Contains(c.View("u"), "1.1.0") || !strings.Contains(c.View("u"), "press u") {
		t.Errorf("available: %q", c.View("u"))
	}
}

func TestKeyStartsTheUpdateAndProgressShows(t *testing.T) {
	c := core()
	c, _ = c.Update(checkedMsg{found: &appkit.Release{Version: "1.1.0"}}, false)
	c, cmd := c.Update(nil, true)
	if !c.Updating() || cmd == nil {
		t.Fatalf("the key did not start the update: %v", c.status)
	}
	c, _ = c.Update(progressMsg(0.42), false)
	if !strings.Contains(c.View("u"), "42%") {
		t.Errorf("progress: %q", c.View("u"))
	}
	// 发布里没有本平台的产物，后台那次更新会失败
	c, _ = c.Update(cmd(), false)
	if c.Updating() || !strings.HasPrefix(c.View("u"), "Update failed") {
		t.Errorf("after the background update: %q", c.View("u"))
	}
}
