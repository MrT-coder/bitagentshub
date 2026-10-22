package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// BenchmarkFrame is one animation step and the full screen it draws, at a
// large terminal: what the office pays every frame.
func BenchmarkFrame(b *testing.B) {
	m, _, _ := twoAgentOffice(&testing.T{})
	m = update(m, tea.WindowSizeMsg{Width: 200, Height: 55})
	b.ReportAllocs()
	for b.Loop() {
		next, _ := m.Update(animMsg{})
		m = next.(Model)
		_ = m.View()
	}
}
