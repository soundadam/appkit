package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOnlyTheKeyStartsTheUpdate(t *testing.T) {
	u := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}
	if !pressed(u, "u") {
		t.Error("u is not the update key")
	}
	if pressed(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}, "u") || pressed(u, "") || pressed("u", "u") {
		t.Error("something other than the key starts the update")
	}
}
