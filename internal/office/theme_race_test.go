package office

import (
	"sync"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// The settings window applies a theme from a background command while the
// office keeps drawing; neither may see half a theme.
func TestSetThemeWhileDrawingIsSafe(t *testing.T) {
	l, c, agents := posedOffice()
	other := BuiltinTheme()
	other.Carpets = other.Carpets[:1]
	other.Agents = other.Agents[:2]
	defer SetTheme(BuiltinTheme())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				SetTheme(other)
			} else {
				SetTheme(BuiltinTheme())
			}
		}
	}()
	for i := 0; i < 200; i++ {
		Draw(screen.New(l.W, l.H/2), l, c, agents, i, "", pet.Sleeping)
		_ = AgentColor("a")
	}
	wg.Wait()
}
