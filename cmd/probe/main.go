// probe measures a cold and a warm poll against the real transcripts and
// prints the snapshot without starting the TUI; "probe render" prints the
// office instead.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/projects"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

func main() {
	home, _ := os.UserHomeDir()
	s := claude.NewStore(filepath.Join(home, ".claude", "projects"), claude.Options{})

	// first is what the office waits for; all is every transcript read, the
	// older ones in steps; warm is one poll after that.
	t0 := time.Now()
	snap := s.Poll(time.Now())
	first := time.Since(t0)
	polls := 1
	for snap.Pending > 0 {
		snap = s.Poll(time.Now())
		polls++
	}
	all := time.Since(t0)
	t1 := time.Now()
	s.Poll(time.Now())
	warm := time.Since(t1)

	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	fmt.Printf("files=%d first=%v all=%v (%d polls) warm=%v heap=%dMB\n", snap.Files, first, all, polls, warm, ms.HeapAlloc>>20)
	fmt.Printf("today=%d week=%d\n", snap.Today.Counted(), snap.Week.Counted())
	for _, a := range snap.Agents {
		fmt.Printf("  %-32s %-8s today=%d session=%d subs=%d\n", a.Project, a.State, a.Today.Counted(), a.Session.Counted(), a.ActiveSubagents)
	}
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == "projects" {
		home, _ := os.UserHomeDir()
		snap := claude.NewStore(filepath.Join(home, ".claude", "projects"), claude.Options{}).Poll(time.Now())
		list := projects.Discover(snap.Projects, nil)
		fmt.Printf("projects=%d (recent=%d)\n", len(list), len(snap.Projects))
		for i, p := range list {
			if i < 8 || i >= len(list)-3 {
				fmt.Printf("  %-34s %-14s recent=%v\n", p.Name, p.Group, p.Recent)
			}
		}
		os.Exit(0)
	}
	if len(os.Args) > 3 && os.Args[1] == "term" {
		// probe term <seconds> <program> [args...] runs it in the office's
		// own pseudo terminal and prints the screen it leaves.
		secs, _ := time.ParseDuration(os.Args[2] + "s")
		dir, _ := os.Getwd()
		notify := make(chan string, 64)
		t0 := time.Now()
		s, err := term.Start("probe", dir, os.Args[3:], 100, 30, notify)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		first := time.Duration(0)
		deadline := time.After(secs)
	wait:
		for {
			select {
			case <-notify:
				if first == 0 {
					first = time.Since(t0)
				}
			case <-deadline:
				break wait
			}
		}
		// PROBE_WHEEL=n scrolls the wheel up n notches and prints the
		// screen before and after, to check full-screen programs scroll.
		if n, _ := strconv.Atoi(os.Getenv("PROBE_WHEEL")); n > 0 {
			before := screen.New(100, 30)
			s.Draw(before, 0, 0, 100, 30, 0, screen.Style{})
			for i := 0; i < n; i++ {
				s.Mouse(term.MouseWheelUp, 50, 10)
				time.Sleep(50 * time.Millisecond)
			}
			time.Sleep(time.Second)
			fmt.Println("--- before wheel ---")
			for _, line := range before.Lines()[:6] {
				fmt.Println(line)
			}
			fmt.Println("--- after wheel ---")
		}
		buf := screen.New(100, 30)
		s.Draw(buf, 0, 0, 100, 30, 0, screen.Style{})
		fmt.Printf("first output after %v, exited=%v, scrollback=%d, altscreen=%v\n", first.Round(time.Millisecond), s.Exited(), s.ScrollbackLen(), s.AltScreen())
		for _, line := range buf.Lines() {
			fmt.Println(line)
		}
		s.Close()
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "herdr" {
		home, _ := os.UserHomeDir()
		snap := claude.NewStore(filepath.Join(home, ".claude", "projects"), claude.Options{}).Poll(time.Now())
		t0 := time.Now()
		panes, err := herdr.NewClient(herdr.DefaultSocketPath()).ListAgents()
		fmt.Printf("herdr panes=%d err=%v took=%v\n", len(panes), err, time.Since(t0))
		links := herdr.Link(snap.Agents, panes)
		for _, a := range snap.Agents {
			p, ok := links[a.ID]
			fmt.Printf("  %-26s linked=%-5v pane=%-8s status=%s\n", a.Project, ok, p.PaneID, p.Status)
		}
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "render" {
		home, _ := os.UserHomeDir()
		snap := claude.NewStore(filepath.Join(home, ".claude", "projects"), claude.Options{}).Poll(time.Now())
		l := office.Build(snap.Agents, 118, 33)
		c := office.NewCrowd(rand.New(rand.NewSource(1)))
		c.Sync(l, snap.Agents)
		for i := 0; i < 300; i++ {
			c.Step(l, snap.Agents)
		}
		buf := screen.New(l.W, l.H/2)
		office.Draw(buf, l, c, snap.Agents, 0, "", pet.Sleeping)
		if len(os.Args) > 2 {
			writePNG(buf, l.W, l.H, os.Args[2])
			os.Exit(0)
		}
		for _, line := range buf.Lines() {
			fmt.Println(line)
		}
		os.Exit(0)
	}
}

// writePNG saves the office pixels scaled 4× so the art can be inspected
// without a terminal. Text cells show as a light mark.
func writePNG(buf *screen.Buffer, w, h int, path string) {
	const s = 4
	img := image.NewRGBA(image.Rect(0, 0, w*s, h*s))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := buf.PixelAt(x, y)
			col := color.RGBA{c.R, c.G, c.B, 255}
			if c == (screen.RGB{}) {
				col = color.RGBA{230, 230, 230, 255}
			}
			for dy := 0; dy < s; dy++ {
				for dx := 0; dx < s; dx++ {
					img.Set(x*s+dx, y*s+dy, col)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
}
