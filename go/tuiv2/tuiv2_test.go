package tuiv2

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestOnlyTheKeyStartsTheUpdate(t *testing.T) {
	u := tea.KeyPressMsg{Code: 'u', Text: "u"}
	if !pressed(u, "u") {
		t.Error("u is not the update key")
	}
	if pressed(tea.KeyPressMsg{Code: 'q', Text: "q"}, "u") || pressed(tea.KeyReleaseMsg{Code: 'u', Text: "u"}, "u") || pressed(u, "") {
		t.Error("something other than the key starts the update")
	}
}
