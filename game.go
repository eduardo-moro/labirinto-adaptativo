package main

import (
	"fmt"
	"math"
	"math/rand"
)

// Params controla a regra probabilística do jogo.
type Params struct {
	L0, LMin, LMax float64 // limiar = P(crescer | movimento não ideal)
	Alpha, Beta    float64 // quanto o limiar sobe no erro / desce no acerto
	Grow           int     // células adicionadas por lado em cada crescimento
	Vision         int     // alcance da visão (passos pelos corredores)
	StartFrac      float64 // tamanho inicial relativo à tela
}

var DefaultParams = Params{
	L0: 0.20, LMin: 0.05, LMax: 0.60,
	Alpha: 0.08, Beta: 0.04,
	Grow: 2, Vision: 3, StartFrac: 0.35,
}

type LogKind int

const (
	LogInfo LogKind = iota
	LogIdeal
	LogError
	LogGrow
	LogWin
)

type LogEntry struct {
	Text string
	Kind LogKind
}

type Game struct {
	P         Params
	M         *Maze
	Seen      [][]bool
	Visits    [][]int
	ChangedAt [][]int
	Dist      [][]int // distância BFS até a saída
	PR, PC    int     // jogador
	ER, EC    int     // saída

	Moves, Ideal, Errors int
	Growths, Regens      int
	ExpGrowths           float64 // Σ P(crescer) nos erros = E[nº de crescimentos]
	SumDeltaErr          int     // Σ Δd nos movimentos não ideais
	Limiar               float64
	LimiarHist, QHist    []float64
	Log                  []LogEntry
	Won                  bool
	rng                  *rand.Rand
}

func NewGame(rows, cols int, p Params, rng *rand.Rand) *Game {
	h := min(rows, max(3, int(float64(rows)*p.StartFrac+0.5)))
	w := min(cols, max(5, int(float64(cols)*p.StartFrac+0.5)))
	r0, c0 := (rows-h)/2, (cols-w)/2
	g := &Game{
		P:         p,
		M:         NewMaze(rows, cols, Rect{r0, c0, r0 + h, c0 + w}),
		Seen:      newGrid[bool](rows, cols),
		Visits:    newGrid[int](rows, cols),
		ChangedAt: newGrid[int](rows, cols),
		Limiar:    p.L0,
		rng:       rng,
	}
	for r := range g.ChangedAt {
		for c := range g.ChangedAt[r] {
			g.ChangedAt[r][c] = -1000
		}
	}
	g.M.Regenerate(g.Seen, rng)
	g.PR, g.PC = r0+h/2, c0+w/2
	g.Visits[g.PR][g.PC] = 1
	g.reveal()
	g.placeExit()
	g.Dist = g.M.BFS(g.ER, g.EC)
	g.pushHist()
	g.addLog(LogInfo, fmt.Sprintf("início: área %d×%d de %d×%d", w, h, cols, rows))
	return g
}

func (g *Game) QHat() float64 {
	if g.Moves == 0 {
		return 0
	}
	return float64(g.Errors) / float64(g.Moves)
}

func (g *Game) MeanDeltaErr() float64 {
	if g.Errors == 0 {
		return 0
	}
	return float64(g.SumDeltaErr) / float64(g.Errors)
}

// ExpectedDelta = E[Δd] = (1-q̂)·(-1) + q̂·E[Δd | erro]
func (g *Game) ExpectedDelta() float64 {
	q := g.QHat()
	return (1-q)*(-1) + q*g.MeanDeltaErr()
}

// Move tenta andar na direção d. Devolve false se há parede.
func (g *Game) Move(d int) bool {
	if g.Won || !g.M.Can(g.PR, g.PC, d) {
		return false
	}
	before := g.Dist[g.PR][g.PC]
	nr, nc := g.M.Neighbor(g.PR, g.PC, d)
	g.PR, g.PC = nr, nc
	g.Moves++
	g.Visits[nr][nc]++
	ideal := g.Dist[nr][nc] == before-1
	g.reveal()

	if nr == g.ER && nc == g.EC {
		g.Ideal++
		g.Won = true
		g.addLog(LogWin, fmt.Sprintf("#%d ★ saída alcançada", g.Moves))
		g.pushHist()
		return true
	}

	if ideal {
		// Jogada ideal: o labirinto NÃO muda, só o limiar diminui.
		g.Ideal++
		g.Limiar = math.Max(g.P.LMin, g.Limiar-g.P.Beta)
		g.addLog(LogIdeal, fmt.Sprintf("#%d ✓ ideal      P→%.2f", g.Moves, g.Limiar))
	} else {
		// Jogada não ideal: limiar sobe, sorteio decide se cresce,
		// e a parte não vista é sempre regenerada.
		g.Errors++
		g.Limiar = math.Min(g.P.LMax, g.Limiar+g.P.Alpha)
		grew := false
		if !g.M.Full() {
			g.ExpGrowths += g.Limiar
			u := g.rng.Float64()
			if u < g.Limiar {
				grew = g.M.Grow(g.P.Grow)
				g.addLog(LogGrow, fmt.Sprintf("#%d ✗ u=%.2f<P=%.2f CRESCE", g.Moves, u, g.Limiar))
			} else {
				g.addLog(LogError, fmt.Sprintf("#%d ✗ u=%.2f≥P=%.2f regen", g.Moves, u, g.Limiar))
			}
		} else {
			g.addLog(LogError, fmt.Sprintf("#%d ✗ área máx.  regen", g.Moves))
		}
		changed := g.M.Regenerate(g.Seen, g.rng)
		g.Regens++
		for _, c := range changed {
			g.ChangedAt[c[0]][c[1]] = g.Moves
		}
		if grew {
			g.Growths++
			g.placeExit()
		}
		g.Dist = g.M.BFS(g.ER, g.EC)
		g.SumDeltaErr += g.Dist[nr][nc] - before
	}
	g.pushHist()
	return true
}

// reveal marca como vistas as células a até Vision passos pelos corredores.
func (g *Game) reveal() {
	type node struct{ r, c, d int }
	g.Seen[g.PR][g.PC] = true
	vis := map[[2]int]bool{{g.PR, g.PC}: true}
	q := []node{{g.PR, g.PC, 0}}
	for i := 0; i < len(q); i++ {
		n := q[i]
		if n.d >= g.P.Vision {
			continue
		}
		for d := 0; d < 4; d++ {
			if !g.M.Can(n.r, n.c, d) {
				continue
			}
			nr, nc := g.M.Neighbor(n.r, n.c, d)
			if vis[[2]int{nr, nc}] {
				continue
			}
			vis[[2]int{nr, nc}] = true
			g.Seen[nr][nc] = true
			q = append(q, node{nr, nc, n.d + 1})
		}
	}
}

// placeExit sorteia a saída na borda da área ativa, de preferência numa
// célula não vista e longe do jogador.
func (g *Game) placeExit() {
	a := g.M.Active
	dp := g.M.BFS(g.PR, g.PC)
	collect := func(border, needUnseen bool) [][2]int {
		var out [][2]int
		for r := a.R0; r < a.R1; r++ {
			for c := a.C0; c < a.C1; c++ {
				if (r == g.PR && c == g.PC) || (border && !a.OnBorder(r, c)) || (needUnseen && g.Seen[r][c]) {
					continue
				}
				out = append(out, [2]int{r, c})
			}
		}
		return out
	}
	cand := collect(true, true)
	if len(cand) == 0 {
		cand = collect(true, false)
	}
	if len(cand) == 0 {
		cand = collect(false, false)
	}
	maxd := 0
	for _, c := range cand {
		maxd = max(maxd, dp[c[0]][c[1]])
	}
	var far [][2]int
	for _, c := range cand {
		if dp[c[0]][c[1]]*10 >= maxd*6 {
			far = append(far, c)
		}
	}
	p := far[g.rng.Intn(len(far))]
	g.ER, g.EC = p[0], p[1]
}

func (g *Game) pushHist() {
	const cap = 300
	g.LimiarHist = append(g.LimiarHist, g.Limiar)
	g.QHist = append(g.QHist, g.QHat())
	if len(g.LimiarHist) > cap {
		g.LimiarHist = g.LimiarHist[1:]
		g.QHist = g.QHist[1:]
	}
}

func (g *Game) addLog(k LogKind, s string) {
	g.Log = append(g.Log, LogEntry{s, k})
	if len(g.Log) > 60 {
		g.Log = g.Log[1:]
	}
}

// OnPath marca o caminho ideal (seguindo a distância BFS) do jogador à saída.
func (g *Game) OnPath() [][]bool {
	p := newGrid[bool](g.M.Rows, g.M.Cols)
	r, c := g.PR, g.PC
	for steps := 0; steps < g.M.Rows*g.M.Cols && g.Dist[r][c] > 0; steps++ {
		p[r][c] = true
		moved := false
		for d := 0; d < 4; d++ {
			if !g.M.Can(r, c, d) {
				continue
			}
			nr, nc := g.M.Neighbor(r, c, d)
			if g.Dist[nr][nc] == g.Dist[r][c]-1 {
				r, c, moved = nr, nc, true
				break
			}
		}
		if !moved {
			break
		}
	}
	p[r][c] = true
	return p
}
