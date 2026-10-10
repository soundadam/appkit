package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	appkit "github.com/soundadam/appkit/go"
)

func updater() Updater {
	app := appkit.App{Name: "demo", Version: "1.0.0", Repo: "owner/demo"}
	return Updater{
		Key:  "u",
		app:  app,
		inst: appkit.Install{Kind: appkit.Executable, Path: "/nonexistent/demo"},
		auto: &appkit.AutoCheck{},
	}
}

func TestQuietUntilSomethingIsFound(t *testing.T) {
	u := updater()
	u, _ = u.Update(checkedMsg{found: nil})
	if u.View() != "" {
		t.Errorf("up to date: %q", u.View())
	}
	u, _ = u.Update(checkedMsg{err: errors.New("offline")})
	if u.View() != "" {
		t.Errorf("a failed automatic check shows %q", u.View())
	}
	u, _ = u.Update(checkedMsg{found: &appkit.Release{Version: "1.1.0"}})
	if !u.Available() || !strings.Contains(u.View(), "1.1.0") || !strings.Contains(u.View(), "press u") {
		t.Errorf("available: %q", u.View())
	}
}

func TestKeyStartsTheUpdateAndProgressShows(t *testing.T) {
	u := updater()
	u, _ = u.Update(checkedMsg{found: &appkit.Release{Version: "1.1.0"}})
	u, cmd := u.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if u.status != updating || cmd == nil {
		t.Fatalf("u did not start the update: %v", u.status)
	}
	u, _ = u.Update(progressMsg(0.42))
	if !strings.Contains(u.View(), "42%") {
		t.Errorf("progress: %q", u.View())
	}
	// 发布里没有本平台的产物，后台那次更新会失败
	u, _ = u.Update(cmd())
	if u.status != failed || !strings.HasPrefix(u.View(), "Update failed") {
		t.Errorf("after the background update: %q", u.View())
	}
}
