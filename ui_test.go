package main

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func dump(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) == 0 {
				sb.WriteByte(' ')
				continue
			}
			sb.WriteString(string(c.Runes))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func newTestApp(t *testing.T, w, h int) (*App, tcell.SimulationScreen) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(w, h)
	a := &App{S: s, BotKind: BotHuman, BotQ: 0.3, SpeedIdx: 2, P: DefaultParams,
		rng: rand.New(rand.NewSource(7))}
	a.W, a.H = w, h
	return a, s
}

func key(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, 0) }

func TestUIFlow(t *testing.T) {
	a, s := newTestApp(t, 150, 42)
	a.draw()
	menu := dump(s)
	if !strings.Contains(menu, "Observar algoritmo") {
		t.Fatal("menu sem opções")
	}
	a.handle(key('2')) // observar
	if a.Mode != ModeObserve || a.G == nil {
		t.Fatal("não entrou no modo observação")
	}
	if a.O.Bot {
		t.Fatal("bot deveria iniciar desligado")
	}
	a.handle(key('9')) // segue caminho ideal
	d0 := a.G.Dist[a.G.PR][a.G.PC]
	for i := 0; i < 5 && !a.G.Won; i++ {
		a.botStep()
	}
	if a.G.Errors != 0 || (!a.G.Won && a.G.Dist[a.G.PR][a.G.PC] != d0-5) {
		t.Fatal("bot com caminho ideal saiu do caminho")
	}
	a.handle(key('9'))
	for i := 0; i < 60; i++ {
		a.botStep()
	}
	a.handle(key('3'))
	a.draw()
	obs := dump(s)

	// redimensionar reinicia
	old := a.G
	s.SetSize(120, 36)
	a.handle(tcell.NewEventResize(120, 36))
	if a.G == old || a.G.M.Cols != ((120-sidebarW)/2-1)/2 || a.G.M.Rows != (36-1-1)/2 {
		t.Fatal("resize não reiniciou com o tamanho da nova tela")
	}
	a.draw()
	resized := dump(s)

	a.handle(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	a.handle(key('1')) // jogar
	for i := 0; i < 15; i++ {
		a.handle(tcell.NewEventKey([]tcell.Key{tcell.KeyUp, tcell.KeyRight, tcell.KeyDown, tcell.KeyLeft}[i%4], 0, 0))
	}
	a.draw()
	play := dump(s)

	// muito pequeno
	s.SetSize(12, 6)
	a.handle(tcell.NewEventResize(12, 6))
	a.draw()

	if os.Getenv("DUMP") != "" {
		os.WriteFile(os.Getenv("DUMP"), []byte(menu+"\n=====\n"+obs+"\n=====\n"+resized+"\n=====\n"+play), 0o644)
	}
}

type countScreen struct {
	tcell.SimulationScreen
	n int
}

func (c *countScreen) Beep() error { c.n++; return nil }

func TestBeepQueue(t *testing.T) {
	a, s := newTestApp(t, 100, 30)
	cs := &countScreen{SimulationScreen: s}
	a.S = cs
	a.Sound = true
	a.beep(3)
	a.beep(3) // fila limitada a 4
	if len(a.beeps) != beepQueue {
		t.Fatalf("fila com %d bipes", len(a.beeps))
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(a.beeps) > 0 && time.Now().Before(deadline) {
		a.flushBeeps()
		if d, ok := a.nextBeep(); ok {
			time.Sleep(d)
		}
	}
	if cs.n != beepQueue {
		t.Fatalf("emitiu %d bipes", cs.n)
	}
	a.Sound = false
	a.beep(3)
	if len(a.beeps) != 0 {
		t.Fatal("bipou com o som desligado")
	}
}
