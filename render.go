package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// Paleta Gruvbox (dark)
const (
	hBg     int32 = 0x1d2021
	hPanel  int32 = 0x282828
	hFloor  int32 = 0x32302f
	hBg1    int32 = 0x3c3836
	hBg2    int32 = 0x504945
	hWall   int32 = 0x7c6f64
	hFg     int32 = 0xebdbb2
	hGray   int32 = 0x928374
	hRed    int32 = 0xfb4934
	hRedD   int32 = 0xcc241d
	hGreen  int32 = 0xb8bb26
	hGreenD int32 = 0x98971a
	hYellow int32 = 0xfabd2f
	hBlue   int32 = 0x83a598
	hPurple int32 = 0xd3869b
	hAqua   int32 = 0x8ec07c
	hOrange int32 = 0xfe8019
	hOrangD int32 = 0xd65d0e
	hMutabl int32 = 0x3b2f3f
)

func col(h int32) tcell.Color { return tcell.NewHexColor(h) }

func sty(fg, bg int32) tcell.Style {
	return tcell.StyleDefault.Foreground(col(fg)).Background(col(bg))
}

func mix(a, b int32, t float64) int32 {
	t = max(0, min(1, t))
	ch := func(s uint) int32 {
		x := float64((a>>s)&0xff)*(1-t) + float64((b>>s)&0xff)*t
		return int32(x+0.5) << s
	}
	return ch(16) | ch(8) | ch(0)
}

// text escreve uma string; caracteres combinantes (ex.: o chapéu de q̂)
// são anexados à célula anterior.
func (a *App) text(x, y int, st tcell.Style, s string) int {
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w == 0 {
			if x > 0 {
				mc, comb, cst, _ := a.S.GetContent(x-1, y)
				a.S.SetContent(x-1, y, mc, append(comb, r), cst)
			}
			continue
		}
		a.S.SetContent(x, y, r, nil, st)
		x += w
	}
	return x
}

func (a *App) fill(x0, y0, x1, y1 int, st tcell.Style) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			a.S.SetContent(x, y, ' ', nil, st)
		}
	}
}

func (a *App) draw() {
	a.S.SetStyle(sty(hFg, hBg))
	a.S.Clear()
	switch a.Mode {
	case ModeMenu:
		a.drawMenu()
	default:
		a.drawGame()
	}
	a.S.Show()
}

// ───────────────────────────── menu ─────────────────────────────

func (a *App) drawMenu() {
	items := []struct{ title, desc string }{
		{"Jogar", "Você nasce no centro. Cada passo que te afasta da saída\nregenera o que você ainda não viu — e pode fazer o labirinto crescer."},
		{"Observar algoritmo", "Um bot joga enquanto o painel mostra em tempo real o caminho\nideal, o limiar de crescimento, q̂, a esperança E[Δd] e as regenerações."},
		{"Sair", ""},
	}
	cx, cy := a.W/2, a.H/2-7
	center := func(y int, st tcell.Style, s string) {
		a.text(cx-runewidth.StringWidth(s)/2, y, st, s)
	}
	center(cy, sty(hYellow, hBg).Bold(true), "L A B I R I N T O   A D A P T A T I V O")
	center(cy+1, sty(hGray, hBg), "estatística aplicada à geração procedural de labirintos")
	y := cy + 4
	for i, it := range items {
		st := sty(hFg, hBg)
		label := "   " + it.title + "   "
		if i == a.MenuSel {
			st = sty(hBg, hYellow).Bold(true)
			label = " ▶ " + it.title + "   "
		}
		center(y, st, label)
		y += 2
	}
	if d := items[a.MenuSel].desc; d != "" {
		for i, l := range strings.Split(d, "\n") {
			center(y+1+i, sty(hBlue, hBg), l)
		}
	}
	center(a.H-2, sty(hGray, hBg), "↑↓ escolher · Enter confirmar · Esc sair")
}

// ───────────────────────────── jogo ─────────────────────────────

func (a *App) drawGame() {
	sw, _, _, ox, oy := a.layout()
	if a.Mode == ModeObserve {
		a.drawSidebar(sw)
	}
	if a.G == nil {
		msg := "Terminal muito pequeno — aumente a janela"
		a.text(sw+(a.W-sw-runewidth.StringWidth(msg))/2, a.H/2, sty(hRed, hBg), msg)
	} else {
		a.drawMaze(ox, oy)
		if a.G.Won {
			a.drawWin(sw)
		}
	}
	a.drawStatus()
}

type mazeCtx struct {
	path    [][]bool
	maxDist int
	maxVis  int
}

func (a *App) drawMaze(ox, oy int) {
	g := a.G
	ctx := mazeCtx{}
	if a.O.Path {
		ctx.path = g.OnPath()
	}
	ar := g.M.Active
	for r := ar.R0; r < ar.R1; r++ {
		for c := ar.C0; c < ar.C1; c++ {
			ctx.maxDist = max(ctx.maxDist, g.Dist[r][c])
			ctx.maxVis = max(ctx.maxVis, g.Visits[r][c])
		}
	}
	for tr := 0; tr < 2*g.M.Rows+1; tr++ {
		for tc := 0; tc < 2*g.M.Cols+1; tc++ {
			st, txt := a.tile(&ctx, tr, tc)
			rs := []rune(txt)
			a.S.SetContent(ox+tc*2, oy+tr, rs[0], nil, st)
			a.S.SetContent(ox+tc*2+1, oy+tr, rs[1], nil, st)
		}
	}
}

func (a *App) active(r, c int) bool {
	m := a.G.M
	return r >= 0 && c >= 0 && r < m.Rows && c < m.Cols && m.Active.Has(r, c)
}

func (a *App) visible(r, c int) bool {
	return a.active(r, c) && (!a.O.Fog || a.G.Seen[r][c])
}

// tile decide o que desenhar em cada posição da grade (2R+1)×(2C+1):
// ímpar/ímpar = célula, ímpar/par ou par/ímpar = parede, par/par = canto.
func (a *App) tile(ctx *mazeCtx, tr, tc int) (tcell.Style, string) {
	m := a.G.M
	outside := func() (tcell.Style, string) {
		if a.O.Bounds {
			return sty(hBg2, hBg), "· "
		}
		return sty(hBg, hBg), "  "
	}
	fog := func() (tcell.Style, string) { return sty(hBg1, hBg), "░░" }
	wall := func() (tcell.Style, string) { return sty(hWall, hWall), "  " }

	ro, co := tr%2 == 1, tc%2 == 1
	switch {
	case ro && co:
		r, c := tr/2, tc/2
		if !a.active(r, c) {
			return outside()
		}
		if !a.visible(r, c) {
			return fog()
		}
		return a.cellTile(ctx, r, c)

	case ro: // parede vertical entre (r,c1) e (r,c2)
		r, c1, c2 := tr/2, tc/2-1, tc/2
		A, B := a.active(r, c1), a.active(r, c2)
		if !A && !B {
			return outside()
		}
		if !a.visible(r, c1) && !a.visible(r, c2) {
			return fog()
		}
		if A && B && m.Can(r, c1, dirE) {
			return a.passageTile(ctx, r, c1, r, c2)
		}
		return wall()

	case co: // parede horizontal entre (r1,c) e (r2,c)
		c, r1, r2 := tc/2, tr/2-1, tr/2
		A, B := a.active(r1, c), a.active(r2, c)
		if !A && !B {
			return outside()
		}
		if !a.visible(r1, c) && !a.visible(r2, c) {
			return fog()
		}
		if A && B && m.Can(r1, c, dirS) {
			return a.passageTile(ctx, r1, c, r2, c)
		}
		return wall()

	default: // canto
		r1, r2, c1, c2 := tr/2-1, tr/2, tc/2-1, tc/2
		if !a.active(r1, c1) && !a.active(r1, c2) && !a.active(r2, c1) && !a.active(r2, c2) {
			return outside()
		}
		if !a.visible(r1, c1) && !a.visible(r1, c2) && !a.visible(r2, c1) && !a.visible(r2, c2) {
			return fog()
		}
		return wall()
	}
}

func (a *App) cellBG(ctx *mazeCtx, r, c int) int32 {
	g := a.G
	bg := hFloor
	if a.O.Mutable && !g.Seen[r][c] {
		bg = hMutabl
	}
	if a.O.Dist && g.Dist[r][c] >= 0 && ctx.maxDist > 0 {
		bg = mix(hGreenD, hOrangD, float64(g.Dist[r][c])/float64(ctx.maxDist))
		bg = mix(bg, hFloor, 0.35)
	}
	if a.O.Heat && g.Visits[r][c] > 0 && ctx.maxVis > 0 {
		bg = mix(bg, hRedD, 0.25+0.65*float64(g.Visits[r][c])/float64(ctx.maxVis))
	}
	if a.O.Regen {
		const fade = 5
		if age := g.Moves - g.ChangedAt[r][c]; age >= 0 && age < fade {
			bg = mix(bg, hPurple, 0.85*(1-float64(age)/fade))
		}
	}
	if ctx.path != nil && ctx.path[r][c] {
		bg = mix(bg, hBlue, 0.8)
	}
	return bg
}

func (a *App) cellTile(ctx *mazeCtx, r, c int) (tcell.Style, string) {
	g := a.G
	if r == g.PR && c == g.PC {
		return sty(hBg, hYellow).Bold(true), "◆ "
	}
	if r == g.ER && c == g.EC {
		return sty(hBg, hGreen).Bold(true), "⚑ "
	}
	bg := a.cellBG(ctx, r, c)
	if a.O.Dist && g.Dist[r][c] >= 0 {
		return sty(hFg, bg), fmt.Sprintf("%2d", g.Dist[r][c]%100)
	}
	return sty(hFg, bg), "  "
}

func (a *App) passageTile(ctx *mazeCtx, r1, c1, r2, c2 int) (tcell.Style, string) {
	v1, v2 := a.visible(r1, c1), a.visible(r2, c2)
	var bg int32
	switch {
	case v1 && v2:
		bg = mix(a.cellBG(ctx, r1, c1), a.cellBG(ctx, r2, c2), 0.5)
	case v1:
		bg = a.cellBG(ctx, r1, c1)
	default:
		bg = a.cellBG(ctx, r2, c2)
	}
	return sty(hFg, bg), "  "
}

// ─────────────────────────── painel ───────────────────────────

func spark(vals []float64, width int, lo, hi float64) string {
	const bars = "▁▂▃▄▅▆▇█"
	br := []rune(bars)
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	var sb strings.Builder
	for _, v := range vals {
		t := (v - lo) / (hi - lo)
		i := int(t*float64(len(br)-1) + 0.5)
		sb.WriteRune(br[max(0, min(len(br)-1, i))])
	}
	return sb.String()
}

func bar(t float64, width int) string {
	n := int(max(0, min(1, t))*float64(width) + 0.5)
	return strings.Repeat("█", n) + strings.Repeat("░", width-n)
}

func (a *App) drawSidebar(sw int) {
	pst := sty(hFg, hPanel)
	a.fill(0, 0, sw-1, a.H-1, pst)
	for y := 0; y < a.H-1; y++ {
		a.S.SetContent(sw-1, y, '│', nil, sty(hBg2, hBg))
	}
	a.optY = map[int]int{}
	y := 0
	line := func(st tcell.Style, s string) {
		if y < a.H-1 {
			a.text(1, y, st, s)
		}
		y++
	}
	header := func(s string) { line(sty(hAqua, hPanel).Bold(true), s) }
	kv := func(label, val string, vst tcell.Style) {
		if y < a.H-1 {
			a.text(1, y, sty(hGray, hPanel), label)
			a.text(sw-2-runewidth.StringWidth(val), y, vst, val)
		}
		y++
	}
	val := sty(hFg, hPanel).Bold(true)

	line(sty(hYellow, hPanel).Bold(true), "LABIRINTO ADAPTATIVO")
	line(sty(hGray, hPanel), "modo observação")
	y++

	header("VISUALIZAÇÃO  (tecla ou clique)")
	for i, o := range optDefs {
		if o.bot {
			continue
		}
		a.optLine(&y, i)
	}
	y++

	header("BOT")
	for i, o := range optDefs {
		if o.bot {
			a.optLine(&y, i)
		}
	}
	kind := a.BotKind.String()
	if a.O.FollowIdeal {
		kind = "caminho ideal"
	} else if a.BotKind == BotHuman {
		kind += fmt.Sprintf(" q=%.2f", a.BotQ)
	}
	kv(" tipo (b, [ ])", kind, val)
	kv(" velocidade (+ -)", speeds[a.SpeedIdx].String(), val)
	state, sst := "▶ rodando", sty(hGreen, hPanel)
	if !a.O.Bot {
		state, sst = "✋ manual", sty(hOrange, hPanel)
	} else if a.Paused {
		state, sst = "⏸ pausado", sty(hYellow, hPanel)
	}
	kv(" estado (espaço, n)", state, sst)
	y++

	g := a.G
	if g == nil {
		return
	}
	header("BERNOULLI  (ideal × não ideal)")
	kv("Movimentos", fmt.Sprint(g.Moves), val)
	kv("Ideais / não ideais", fmt.Sprintf("%d / %d", g.Ideal, g.Errors), val)
	kv("q̂ = não ideais / mov.", fmt.Sprintf("%.3f", g.QHat()), sty(hOrange, hPanel).Bold(true))
	kv("q̂", spark(g.QHist, sw-8, 0, 1), sty(hOrange, hPanel))
	y++

	header("LIMIAR  P(crescer | erro)")
	kv(fmt.Sprintf("P = %.2f", g.Limiar), bar((g.Limiar-g.P.LMin)/(g.P.LMax-g.P.LMin), 18), sty(hPurple, hPanel))
	kv(fmt.Sprintf("mín %.2f  máx %.2f", g.P.LMin, g.P.LMax), fmt.Sprintf("+%.2f / −%.2f", g.P.Alpha, g.P.Beta), sty(hGray, hPanel))
	kv("P", spark(g.LimiarHist, sw-8, g.P.LMin, g.P.LMax), sty(hPurple, hPanel))
	kv("Crescimentos", fmt.Sprint(g.Growths), val)
	kv("E[cresc.] = Σ P", fmt.Sprintf("%.2f", g.ExpGrowths), sty(hPurple, hPanel).Bold(true))
	kv("Regenerações", fmt.Sprint(g.Regens), val)
	kv("Área", fmt.Sprintf("%d×%d de %d×%d", g.M.Active.W(), g.M.Active.H(), g.M.Cols, g.M.Rows), val)
	y++

	header("ESPERANÇA  E[Δd]")
	kv("Distância até a saída", fmt.Sprint(g.Dist[g.PR][g.PC]), val)
	kv("Δd médio no erro", fmt.Sprintf("%+.2f", g.MeanDeltaErr()), val)
	line(sty(hGray, hPanel), "(1−q̂)·(−1) + q̂·Δd_erro")
	ed := g.ExpectedDelta()
	es, est := "↓ aproximando", sty(hGreen, hPanel).Bold(true)
	if ed >= 0 {
		es, est = "↑ afastando", sty(hRed, hPanel).Bold(true)
	}
	kv(fmt.Sprintf("  = %+.3f", ed), es, est)
	y++

	header("EVENTOS")
	n := max(0, a.H-1-y)
	logs := g.Log
	if len(logs) > n {
		logs = logs[len(logs)-n:]
	}
	for i := len(logs) - 1; i >= 0; i-- {
		e := logs[i]
		c := map[LogKind]int32{LogInfo: hGray, LogIdeal: hGreen, LogError: hOrange, LogGrow: hPurple, LogWin: hYellow}[e.Kind]
		line(sty(c, hPanel), e.Text)
	}
}

func (a *App) optLine(y *int, i int) {
	o := optDefs[i]
	on := *o.ptr(&a.O)
	box, bst := "[ ]", sty(hGray, hPanel)
	if on {
		box, bst = "[x]", sty(hGreen, hPanel).Bold(true)
	}
	if *y < a.H-1 {
		x := a.text(1, *y, bst, box)
		x = a.text(x+1, *y, sty(hYellow, hPanel), string(o.key))
		a.text(x+1, *y, sty(hFg, hPanel), o.label)
		a.optY[*y] = i
	}
	*y++
}

// ─────────────────────────── status / vitória ───────────────────────────

func (a *App) drawStatus() {
	y := a.H - 1
	st := sty(hFg, hBg1)
	a.fill(0, y, a.W, y+1, st)
	var s string
	if a.Mode == ModePlay {
		s = " ←↑↓→ / WASD mover · r reinicia · Esc menu"
		if a.G != nil {
			s = fmt.Sprintf(" Movimentos %d  ·%s", a.G.Moves, s)
		}
	} else {
		s = " 1-9 opções · espaço pausa · n passo · b bot · [ ] q · + - velocidade · setas controle manual · r reinicia · Esc menu"
	}
	if time.Now().Before(a.flashUntil) {
		s = " ⚠ " + a.flashMsg
		st = sty(hBg, hYellow).Bold(true)
		a.fill(0, y, a.W, y+1, st)
	}
	a.text(0, y, st, s)
}

func (a *App) drawWin(sw int) {
	g := a.G
	title := "★  Você encontrou a saída!  ★"
	foot := "Enter: novo labirinto · Esc: menu"
	if a.Mode == ModeObserve {
		if a.O.Bot {
			title = "★  O bot encontrou a saída  ★"
			foot = "reiniciando..."
		}
	}
	lines := []string{
		fmt.Sprintf("Movimentos            %6d", g.Moves),
		fmt.Sprintf("Ideais / não ideais   %3d / %d", g.Ideal, g.Errors),
		fmt.Sprintf("q̂ (taxa de erro)       %6.3f", g.QHat()),
		fmt.Sprintf("Crescimentos          %6d", g.Growths),
		fmt.Sprintf("E[crescimentos] = ΣP  %6.2f", g.ExpGrowths),
		fmt.Sprintf("Regenerações          %6d", g.Regens),
		fmt.Sprintf("Área final            %d×%d", g.M.Active.W(), g.M.Active.H()),
	}
	w := 40
	h := len(lines) + 6
	x0 := sw + (a.W-sw-w)/2
	y0 := (a.H - 1 - h) / 2
	a.fill(x0, y0, x0+w, y0+h, sty(hFg, hPanel))
	for x := x0; x < x0+w; x++ {
		a.S.SetContent(x, y0, '─', nil, sty(hYellow, hPanel))
		a.S.SetContent(x, y0+h-1, '─', nil, sty(hYellow, hPanel))
	}
	a.text(x0+(w-runewidth.StringWidth(title))/2, y0+1, sty(hYellow, hPanel).Bold(true), title)
	for i, l := range lines {
		a.text(x0+3, y0+3+i, sty(hFg, hPanel), l)
	}
	a.text(x0+(w-runewidth.StringWidth(foot))/2, y0+h-2, sty(hGray, hPanel), foot)
}
