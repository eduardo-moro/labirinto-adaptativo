package main

import (
	"math/rand"
	"testing"
)

// checkTree: a área ativa precisa ser um labirinto perfeito
// (conexo e com exatamente N-1 passagens) e não pode vazar para fora dela.
func checkTree(t *testing.T, m *Maze) {
	t.Helper()
	a := m.Active
	edges, cells := 0, a.H()*a.W()
	for r := 0; r < m.Rows; r++ {
		for c := 0; c < m.Cols; c++ {
			if !a.Has(r, c) {
				if m.Open[r][c] != 0 {
					t.Fatalf("célula fora da área ativa com passagem: %d,%d", r, c)
				}
				continue
			}
			for d := 0; d < 4; d++ {
				if !m.Can(r, c, d) {
					continue
				}
				nr, nc := m.Neighbor(r, c, d)
				if !a.Has(nr, nc) {
					t.Fatalf("passagem saindo da área ativa em %d,%d", r, c)
				}
				if m.Open[nr][nc]&dirs[d].opp == 0 {
					t.Fatalf("passagem assimétrica em %d,%d", r, c)
				}
				if d == dirS || d == dirE {
					edges++
				}
			}
		}
	}
	if edges != cells-1 {
		t.Fatalf("não é árvore: %d passagens para %d células", edges, cells)
	}
	dist := m.BFS(a.R0, a.C0)
	for r := a.R0; r < a.R1; r++ {
		for c := a.C0; c < a.C1; c++ {
			if dist[r][c] < 0 {
				t.Fatalf("célula inalcançável %d,%d", r, c)
			}
		}
	}
}

func TestInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for game := 0; game < 200; game++ {
		g := NewGame(15, 30, DefaultParams, rng)
		checkTree(t, g.M)
		kind := BotKind(game % int(botCount))
		for step := 0; step < 3000 && !g.Won; step++ {
			// guarda o estado das células vistas antes do movimento
			snap := map[[2]int]uint8{}
			for r := range g.Seen {
				for c := range g.Seen[r] {
					if g.Seen[r][c] {
						snap[[2]int{r, c}] = g.M.Open[r][c]
					}
				}
			}
			before := g.Dist[g.PR][g.PC]
			d := g.BotDir(kind, 0.4)
			if d < 0 {
				t.Fatal("bot sem movimento")
			}
			errs := g.Errors
			g.Move(d)
			checkTree(t, g.M)
			if g.Dist[g.ER][g.EC] != 0 || !g.M.Active.Has(g.ER, g.EC) {
				t.Fatal("saída inválida")
			}
			if g.Errors == errs && !g.Won && g.Dist[g.PR][g.PC] != before-1 {
				t.Fatal("movimento ideal não reduziu a distância em 1")
			}
			grew := false
			for k, v := range snap {
				if g.M.Open[k[0]][k[1]] != v {
					// só é permitido abrir para a área nova (borda antiga) — nunca fechar
					if g.M.Open[k[0]][k[1]]&v != v {
						t.Fatalf("célula vista perdeu passagem em %v", k)
					}
					grew = true
				}
			}
			_ = grew
		}
	}
}
