package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
)

type Mode int

const (
	ModeMenu Mode = iota
	ModePlay
	ModeObserve
)

type Opts struct {
	Fog, Path, Dist, Mutable, Regen, Heat, Bounds, Bot, FollowIdeal bool
}

type optDef struct {
	key   rune
	label string
	ptr   func(*Opts) *bool
	bot   bool // aparece na seção BOT do painel
}

var optDefs = []optDef{
	{key: '1', label: "Névoa", ptr: func(o *Opts) *bool { return &o.Fog }},
	{key: '2', label: "Caminho ideal (BFS)", ptr: func(o *Opts) *bool { return &o.Path }},
	{key: '3', label: "Distância até a saída", ptr: func(o *Opts) *bool { return &o.Dist }},
	{key: '4', label: "Região mutável (não vista)", ptr: func(o *Opts) *bool { return &o.Mutable }},
	{key: '5', label: "Células regeneradas", ptr: func(o *Opts) *bool { return &o.Regen }},
	{key: '6', label: "Mapa de calor de visitas", ptr: func(o *Opts) *bool { return &o.Heat }},
	{key: '7', label: "Limite máximo (tela)", ptr: func(o *Opts) *bool { return &o.Bounds }},
	{key: '8', label: "Bot automático", ptr: func(o *Opts) *bool { return &o.Bot }, bot: true},
	{key: '9', label: "Bot segue caminho ideal", ptr: func(o *Opts) *bool { return &o.FollowIdeal }, bot: true},
}

var speeds = []time.Duration{
	600 * time.Millisecond, 300 * time.Millisecond, 150 * time.Millisecond,
	80 * time.Millisecond, 40 * time.Millisecond, 15 * time.Millisecond,
}

const sidebarW = 38

type App struct {
	S        tcell.Screen
	Mode     Mode
	MenuSel  int
	G        *Game
	O        Opts
	BotKind  BotKind
	BotQ     float64
	Paused   bool
	SpeedIdx int
	P        Params
	CSV      string

	W, H       int
	nextTick   time.Time
	wonAt      time.Time
	saved      bool
	optY       map[int]int
	lastBtn    tcell.ButtonMask
	flashMsg   string
	flashUntil time.Time
	rng        *rand.Rand
}

func main() {
	sim := flag.Int("sim", 0, "roda N partidas sem interface e imprime estatísticas")
	bot := flag.String("bot", "humano", "bot da simulação: perfeito | humano | aleatorio")
	q := flag.Float64("q", 0.30, "probabilidade de erro do bot humano")
	rows := flag.Int("linhas", 21, "linhas do labirinto na simulação")
	cols := flag.Int("colunas", 34, "colunas do labirinto na simulação")
	maxMoves := flag.Int("max", 200000, "limite de movimentos por partida na simulação")
	csvPath := flag.String("csv", "", "arquivo CSV para salvar os resultados das partidas")
	flag.Parse()

	if *sim > 0 {
		k, ok := parseBot(*bot)
		if !ok {
			fmt.Fprintln(os.Stderr, "bot inválido:", *bot)
			os.Exit(2)
		}
		if err := runSim(*sim, k, *q, *rows, *cols, *maxMoves, *csvPath, DefaultParams); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro ao iniciar o terminal:", err)
		os.Exit(1)
	}
	defer s.Fini()
	s.EnableMouse(tcell.MouseButtonEvents)
	s.HideCursor()

	a := &App{
		S: s, BotKind: BotHuman, BotQ: *q, SpeedIdx: 2, P: DefaultParams, CSV: *csvPath,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	a.W, a.H = s.Size()
	a.run()
}

func (a *App) run() {
	events := make(chan tcell.Event, 16)
	go func() {
		for {
			ev := a.S.PollEvent()
			if ev == nil {
				return
			}
			events <- ev
		}
	}()
	for {
		a.draw()
		var timer <-chan time.Time
		if d, ok := a.nextWake(); ok {
			timer = time.After(d)
		}
		select {
		case ev := <-events:
			if !a.handle(ev) {
				return
			}
		case <-timer:
			a.tick()
		}
	}
}

func (a *App) botActive() bool {
	return a.Mode == ModeObserve && a.O.Bot && !a.Paused && a.G != nil
}

func (a *App) nextWake() (time.Duration, bool) {
	now := time.Now()
	if !a.flashUntil.IsZero() && now.Before(a.flashUntil) && !a.botActive() {
		return a.flashUntil.Sub(now), true
	}
	if !a.botActive() {
		return 0, false
	}
	t := a.nextTick
	if a.G.Won {
		t = a.wonAt.Add(2500 * time.Millisecond)
	}
	return max(0, t.Sub(now)), true
}

func (a *App) tick() {
	if !a.botActive() {
		return
	}
	if a.G.Won {
		if time.Since(a.wonAt) >= 2500*time.Millisecond {
			a.newGame()
		}
		return
	}
	if time.Now().Before(a.nextTick) {
		return
	}
	a.botStep()
	a.nextTick = time.Now().Add(speeds[a.SpeedIdx])
}

func (a *App) botStep() {
	if a.G == nil || a.G.Won {
		return
	}
	kind := a.BotKind
	if a.O.FollowIdeal {
		kind = BotPerfect
	}
	if d := a.G.BotDir(kind, a.BotQ); d >= 0 {
		a.move(d)
	}
}

func (a *App) move(d int) {
	if a.G.Move(d) && a.G.Won {
		a.wonAt = time.Now()
		if a.CSV != "" && !a.saved {
			mode, bot := "jogador", "-"
			if a.Mode == ModeObserve {
				mode = "observacao"
				if a.O.Bot {
					bot = a.BotKind.String()
					if a.O.FollowIdeal {
						bot = BotPerfect.String()
					}
				}
			}
			if err := appendCSV(a.CSV, mode, bot, a.BotQ, a.G); err != nil {
				a.flash("erro ao salvar CSV: " + err.Error())
			}
			a.saved = true
		}
	}
}

// layout: o labirinto máximo é o que cabe na tela (cada célula = 2×2 tiles,
// cada tile = 2 colunas).
func (a *App) layout() (sw, rows, cols, ox, oy int) {
	if a.Mode == ModeObserve {
		sw = sidebarW
	}
	availW, availH := a.W-sw, a.H-1
	cols = (availW/2 - 1) / 2
	rows = (availH - 1) / 2
	ox = sw + (availW-(2*cols+1)*2)/2
	oy = (availH - (2*rows + 1)) / 2
	return
}

func (a *App) newGame() {
	_, rows, cols, _, _ := a.layout()
	a.saved = false
	if rows < 3 || cols < 5 {
		a.G = nil
		return
	}
	a.G = NewGame(rows, cols, a.P, a.rng)
	a.nextTick = time.Now().Add(speeds[a.SpeedIdx])
}

func (a *App) start(m Mode) {
	a.Mode = m
	a.Paused = false
	if m == ModePlay {
		a.O = Opts{Fog: true}
	} else {
		a.O = Opts{Fog: true, Path: true, Regen: true, Mutable: true} // bot começa desligado
	}
	a.newGame()
}

func (a *App) flash(s string) {
	a.flashMsg = s
	a.flashUntil = time.Now().Add(3 * time.Second)
}

func (a *App) handle(ev tcell.Event) bool {
	switch e := ev.(type) {
	case *tcell.EventResize:
		a.S.Sync()
		w, h := e.Size()
		if w != a.W || h != a.H {
			a.W, a.H = w, h
			if a.Mode != ModeMenu {
				a.newGame()
				a.flash(fmt.Sprintf("tela redimensionada (%d×%d) → jogo reiniciado", w, h))
			}
		}
	case *tcell.EventKey:
		return a.key(e)
	case *tcell.EventMouse:
		btn := e.Buttons()
		if btn&tcell.Button1 != 0 && a.lastBtn&tcell.Button1 == 0 && a.Mode == ModeObserve {
			x, y := e.Position()
			if x < sidebarW {
				if i, ok := a.optY[y]; ok {
					a.toggle(i)
				}
			}
		}
		a.lastBtn = btn
	}
	return true
}

func (a *App) toggle(i int) {
	p := optDefs[i].ptr(&a.O)
	*p = !*p
	if p == &a.O.Bot && a.O.Bot {
		a.Paused = false
		a.nextTick = time.Now()
	}
}

func keyDir(e *tcell.EventKey) int {
	switch e.Key() {
	case tcell.KeyUp:
		return dirN
	case tcell.KeyDown:
		return dirS
	case tcell.KeyRight:
		return dirE
	case tcell.KeyLeft:
		return dirW
	case tcell.KeyRune:
		switch e.Rune() {
		case 'w', 'W', 'k':
			return dirN
		case 's', 'S', 'j':
			return dirS
		case 'd', 'D', 'l':
			return dirE
		case 'a', 'A', 'h':
			return dirW
		}
	}
	return -1
}

func (a *App) key(e *tcell.EventKey) bool {
	if e.Key() == tcell.KeyCtrlC {
		return false
	}
	if a.Mode == ModeMenu {
		switch e.Key() {
		case tcell.KeyUp:
			a.MenuSel = (a.MenuSel + 2) % 3
		case tcell.KeyDown, tcell.KeyTab:
			a.MenuSel = (a.MenuSel + 1) % 3
		case tcell.KeyEnter:
			return a.menuPick(a.MenuSel)
		case tcell.KeyEscape:
			return false
		case tcell.KeyRune:
			switch e.Rune() {
			case '1':
				return a.menuPick(0)
			case '2':
				return a.menuPick(1)
			case 'q', '3':
				return false
			}
		}
		return true
	}

	switch e.Key() {
	case tcell.KeyEscape:
		a.Mode = ModeMenu
		return true
	case tcell.KeyEnter:
		if a.G == nil || a.G.Won {
			a.newGame()
		}
		return true
	}
	if e.Key() == tcell.KeyRune && (e.Rune() == 'r' || e.Rune() == 'R') {
		a.newGame()
		return true
	}
	if a.G == nil {
		return true
	}
	if d := keyDir(e); d >= 0 {
		if a.Mode == ModeObserve && a.O.Bot {
			a.O.Bot = false // assume o controle manual
			a.flash("bot desligado: controle manual")
		}
		a.move(d)
		return true
	}
	if a.Mode != ModeObserve || e.Key() != tcell.KeyRune {
		return true
	}
	switch r := e.Rune(); r {
	case ' ':
		a.Paused = !a.Paused
		a.nextTick = time.Now()
	case 'n', 'N':
		a.botStep()
	case 'b', 'B':
		a.BotKind = (a.BotKind + 1) % botCount
	case '[':
		a.BotQ = max(0, a.BotQ-0.05)
	case ']':
		a.BotQ = min(1, a.BotQ+0.05)
	case '+', '=':
		a.SpeedIdx = min(len(speeds)-1, a.SpeedIdx+1)
	case '-', '_':
		a.SpeedIdx = max(0, a.SpeedIdx-1)
	default:
		for i, o := range optDefs {
			if o.key == r {
				a.toggle(i)
			}
		}
	}
	return true
}

func (a *App) menuPick(i int) bool {
	switch i {
	case 0:
		a.start(ModePlay)
	case 1:
		a.start(ModeObserve)
	default:
		return false
	}
	return true
}
