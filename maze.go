package main

import "math/rand"

// Cada célula guarda um bitmask das passagens abertas (N, S, L, O).
const (
	bitN uint8 = 1 << iota
	bitS
	bitE
	bitW
)

const (
	dirN = iota
	dirS
	dirE
	dirW
)

type dirInfo struct {
	dr, dc   int
	bit, opp uint8
}

var dirs = [4]dirInfo{
	{-1, 0, bitN, bitS},
	{1, 0, bitS, bitN},
	{0, 1, bitE, bitW},
	{0, -1, bitW, bitE},
}

// Rect é um retângulo de células [R0,R1) x [C0,C1).
type Rect struct{ R0, C0, R1, C1 int }

func (r Rect) Has(rr, cc int) bool {
	return rr >= r.R0 && rr < r.R1 && cc >= r.C0 && cc < r.C1
}
func (r Rect) H() int { return r.R1 - r.R0 }
func (r Rect) W() int { return r.C1 - r.C0 }
func (r Rect) OnBorder(rr, cc int) bool {
	return rr == r.R0 || rr == r.R1-1 || cc == r.C0 || cc == r.C1-1
}

// Maze é um labirinto perfeito (árvore geradora) sobre a área ativa.
// Rows x Cols é o tamanho máximo (definido pela tela).
type Maze struct {
	Rows, Cols int
	Open       [][]uint8
	Active     Rect
}

func newGrid[T any](rows, cols int) [][]T {
	g := make([][]T, rows)
	for i := range g {
		g[i] = make([]T, cols)
	}
	return g
}

func NewMaze(rows, cols int, active Rect) *Maze {
	return &Maze{Rows: rows, Cols: cols, Open: newGrid[uint8](rows, cols), Active: active}
}

func (m *Maze) Can(r, c, d int) bool { return m.Open[r][c]&dirs[d].bit != 0 }

func (m *Maze) Neighbor(r, c, d int) (int, int) { return r + dirs[d].dr, c + dirs[d].dc }

func (m *Maze) setLink(r, c, d int, open bool) {
	nr, nc := m.Neighbor(r, c, d)
	if open {
		m.Open[r][c] |= dirs[d].bit
		m.Open[nr][nc] |= dirs[d].opp
	} else {
		m.Open[r][c] &^= dirs[d].bit
		m.Open[nr][nc] &^= dirs[d].opp
	}
}

func (m *Maze) Full() bool { return m.Active == Rect{0, 0, m.Rows, m.Cols} }

// Grow expande a área ativa em g células para cada lado, limitada à tela.
func (m *Maze) Grow(g int) bool {
	a := m.Active
	na := Rect{max(0, a.R0-g), max(0, a.C0-g), min(m.Rows, a.R1+g), min(m.Cols, a.C1+g)}
	if na == a {
		return false
	}
	m.Active = na
	return true
}

type unionFind []int

func newUF(n int) unionFind {
	u := make(unionFind, n)
	for i := range u {
		u[i] = i
	}
	return u
}

func (u unionFind) find(x int) int {
	for u[x] != x {
		u[x] = u[u[x]]
		x = u[x]
	}
	return x
}

func (u unionFind) union(a, b int) bool {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return false
	}
	u[ra] = rb
	return true
}

// Regenerate refaz aleatoriamente toda a parte NÃO VISTA do labirinto,
// mantendo intactas as paredes/passagens de células que o jogador já viu.
// O resultado continua sendo um labirinto perfeito (uma árvore): existe
// exatamente um caminho entre quaisquer duas células.
//
//  1. apaga as passagens entre células não vistas;
//  2. union-find com as passagens que sobraram (as fixas);
//  3. DFS aleatório (backtracker) sobre as células não vistas → corredores longos;
//  4. Kruskal aleatório conecta o que sobrou separado.
//
// Devolve as células cujo formato mudou.
func (m *Maze) Regenerate(seen [][]bool, rng *rand.Rand) [][2]int {
	a := m.Active
	before := newGrid[uint8](m.Rows, m.Cols)
	for r := range m.Open {
		copy(before[r], m.Open[r])
	}
	idx := func(r, c int) int { return r*m.Cols + c }

	// 1. limpa
	for r := a.R0; r < a.R1; r++ {
		for c := a.C0; c < a.C1; c++ {
			for d := 0; d < 4; d++ {
				nr, nc := m.Neighbor(r, c, d)
				if !a.Has(nr, nc) {
					m.Open[r][c] &^= dirs[d].bit
					continue
				}
				if !seen[r][c] && !seen[nr][nc] {
					m.setLink(r, c, d, false)
				}
			}
		}
	}

	// 2. componentes já existentes
	u := newUF(m.Rows * m.Cols)
	for r := a.R0; r < a.R1; r++ {
		for c := a.C0; c < a.C1; c++ {
			for _, d := range []int{dirS, dirE} {
				nr, nc := m.Neighbor(r, c, d)
				if a.Has(nr, nc) && m.Can(r, c, d) && !u.union(idx(r, c), idx(nr, nc)) {
					m.setLink(r, c, d, false) // segurança: nunca deixar ciclo
				}
			}
		}
	}

	// 3. DFS aleatório nas células não vistas
	var cells [][2]int
	for r := a.R0; r < a.R1; r++ {
		for c := a.C0; c < a.C1; c++ {
			if !seen[r][c] {
				cells = append(cells, [2]int{r, c})
			}
		}
	}
	rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	visited := newGrid[bool](m.Rows, m.Cols)
	for _, s := range cells {
		if visited[s[0]][s[1]] {
			continue
		}
		visited[s[0]][s[1]] = true
		stack := [][2]int{s}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			var opts [4]int
			n := 0
			for d := 0; d < 4; d++ {
				nr, nc := m.Neighbor(cur[0], cur[1], d)
				if a.Has(nr, nc) && !seen[nr][nc] && !visited[nr][nc] &&
					u.find(idx(cur[0], cur[1])) != u.find(idx(nr, nc)) {
					opts[n] = d
					n++
				}
			}
			if n == 0 {
				stack = stack[:len(stack)-1]
				continue
			}
			d := opts[rng.Intn(n)]
			nr, nc := m.Neighbor(cur[0], cur[1], d)
			m.setLink(cur[0], cur[1], d, true)
			u.union(idx(cur[0], cur[1]), idx(nr, nc))
			visited[nr][nc] = true
			stack = append(stack, [2]int{nr, nc})
		}
	}

	// 4. Kruskal: prioriza paredes invisíveis; só abre parede já vista em último caso
	type edge struct{ r, c, d int }
	var tiers [3][]edge
	for r := a.R0; r < a.R1; r++ {
		for c := a.C0; c < a.C1; c++ {
			for _, d := range []int{dirS, dirE} {
				nr, nc := m.Neighbor(r, c, d)
				if !a.Has(nr, nc) || m.Can(r, c, d) {
					continue
				}
				t := 0
				if seen[r][c] {
					t++
				}
				if seen[nr][nc] {
					t++
				}
				tiers[t] = append(tiers[t], edge{r, c, d})
			}
		}
	}
	for _, tier := range tiers {
		rng.Shuffle(len(tier), func(i, j int) { tier[i], tier[j] = tier[j], tier[i] })
		for _, e := range tier {
			nr, nc := m.Neighbor(e.r, e.c, e.d)
			if u.union(idx(e.r, e.c), idx(nr, nc)) {
				m.setLink(e.r, e.c, e.d, true)
			}
		}
	}

	var changed [][2]int
	for r := a.R0; r < a.R1; r++ {
		for c := a.C0; c < a.C1; c++ {
			if before[r][c] != m.Open[r][c] {
				changed = append(changed, [2]int{r, c})
			}
		}
	}
	return changed
}

// BFS devolve a distância (em passos) de toda célula até (sr,sc); -1 = inalcançável.
func (m *Maze) BFS(sr, sc int) [][]int {
	d := newGrid[int](m.Rows, m.Cols)
	for r := range d {
		for c := range d[r] {
			d[r][c] = -1
		}
	}
	d[sr][sc] = 0
	q := [][2]int{{sr, sc}}
	for i := 0; i < len(q); i++ {
		r, c := q[i][0], q[i][1]
		for k := 0; k < 4; k++ {
			if !m.Can(r, c, k) {
				continue
			}
			nr, nc := m.Neighbor(r, c, k)
			if d[nr][nc] >= 0 {
				continue
			}
			d[nr][nc] = d[r][c] + 1
			q = append(q, [2]int{nr, nc})
		}
	}
	return d
}
