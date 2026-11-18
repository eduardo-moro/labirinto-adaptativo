package main

import "strings"

type BotKind int

const (
	BotPerfect BotKind = iota
	BotHuman
	BotRandom
	botCount
)

func (k BotKind) String() string {
	return [...]string{"Perfeito", "Humano", "Aleatório"}[k]
}

func parseBot(s string) (BotKind, bool) {
	switch strings.ToLower(s) {
	case "perfeito", "perfect":
		return BotPerfect, true
	case "humano", "human":
		return BotHuman, true
	case "aleatorio", "aleatório", "random":
		return BotRandom, true
	}
	return 0, false
}

// BotDir escolhe a próxima direção:
//   - Perfeito: sempre o vizinho ideal (q = 0)
//   - Humano: com probabilidade q escolhe um vizinho não ideal (se existir)
//   - Aleatório: passeio aleatório, cada vizinho com P = 1/k
func (g *Game) BotDir(k BotKind, q float64) int {
	ideal := -1
	var others []int
	cur := g.Dist[g.PR][g.PC]
	for d := 0; d < 4; d++ {
		if !g.M.Can(g.PR, g.PC, d) {
			continue
		}
		nr, nc := g.M.Neighbor(g.PR, g.PC, d)
		if ideal < 0 && g.Dist[nr][nc] == cur-1 {
			ideal = d
		} else {
			others = append(others, d)
		}
	}
	switch k {
	case BotHuman:
		if len(others) > 0 && (ideal < 0 || g.rng.Float64() < q) {
			return others[g.rng.Intn(len(others))]
		}
	case BotRandom:
		all := others
		if ideal >= 0 {
			all = append(all, ideal)
		}
		if len(all) > 0 {
			return all[g.rng.Intn(len(all))]
		}
	}
	if ideal >= 0 {
		return ideal
	}
	if len(others) > 0 {
		return others[0]
	}
	return -1
}
