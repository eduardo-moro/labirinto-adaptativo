package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"
)

var csvHeader = []string{
	"data", "modo", "bot", "q_bot", "linhas", "colunas", "movimentos", "ideais", "nao_ideais",
	"q_hat", "crescimentos", "crescimentos_esperados", "regeneracoes",
	"area_final_linhas", "area_final_colunas", "delta_medio_erro", "venceu",
}

func appendCSV(path, mode, bot string, q float64, g *Game) error {
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if os.IsNotExist(statErr) {
		w.Write(csvHeader)
	}
	ff := func(x float64) string { return strconv.FormatFloat(x, 'f', 4, 64) }
	w.Write([]string{
		time.Now().Format(time.RFC3339), mode, bot, ff(q),
		strconv.Itoa(g.M.Rows), strconv.Itoa(g.M.Cols),
		strconv.Itoa(g.Moves), strconv.Itoa(g.Ideal), strconv.Itoa(g.Errors),
		ff(g.QHat()), strconv.Itoa(g.Growths), ff(g.ExpGrowths), strconv.Itoa(g.Regens),
		strconv.Itoa(g.M.Active.H()), strconv.Itoa(g.M.Active.W()),
		ff(g.MeanDeltaErr()), strconv.FormatBool(g.Won),
	})
	w.Flush()
	return w.Error()
}

type stat struct{ xs []float64 }

func (s *stat) add(x float64) { s.xs = append(s.xs, x) }
func (s *stat) mean() float64 {
	t := 0.0
	for _, x := range s.xs {
		t += x
	}
	return t / float64(len(s.xs))
}
func (s *stat) sd() float64 {
	if len(s.xs) < 2 {
		return 0
	}
	m, t := s.mean(), 0.0
	for _, x := range s.xs {
		t += (x - m) * (x - m)
	}
	return math.Sqrt(t / float64(len(s.xs)-1))
}

// runSim joga n partidas sem interface e imprime média, desvio e IC 95%.
func runSim(n int, kind BotKind, q float64, rows, cols, maxMoves int, csvPath string, p Params) error {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var moves, qhat, grow, expGrow, area, dErr, wins stat
	for i := 0; i < n; i++ {
		g := NewGame(rows, cols, p, rng)
		for !g.Won && g.Moves < maxMoves {
			d := g.BotDir(kind, q)
			if d < 0 {
				break
			}
			g.Move(d)
		}
		moves.add(float64(g.Moves))
		qhat.add(g.QHat())
		grow.add(float64(g.Growths))
		expGrow.add(g.ExpGrowths)
		area.add(float64(g.M.Active.H() * g.M.Active.W()))
		dErr.add(g.MeanDeltaErr())
		if g.Won {
			wins.add(1)
		} else {
			wins.add(0)
		}
		if csvPath != "" {
			if err := appendCSV(csvPath, "simulacao", kind.String(), q, g); err != nil {
				return err
			}
		}
	}
	fmt.Printf("Simulação: %d partidas · bot %s (q=%.2f) · labirinto máx. %d×%d\n\n", n, kind, q, cols, rows)
	fmt.Printf("%-26s %10s %10s %22s\n", "métrica", "média", "desvio", "IC 95% (média ± 1,96·s/√n)")
	row := func(name string, s *stat) {
		m, sd := s.mean(), s.sd()
		e := 1.96 * sd / math.Sqrt(float64(len(s.xs)))
		fmt.Printf("%-26s %10.3f %10.3f %10.3f – %-10.3f\n", name, m, sd, m-e, m+e)
	}
	row("movimentos até a saída", &moves)
	row("q̂ (taxa de não ideais)", &qhat)
	row("crescimentos observados", &grow)
	row("E[crescimentos] = ΣP", &expGrow)
	row("área final (células)", &area)
	row("Δd médio no erro", &dErr)
	row("taxa de vitória", &wins)
	if csvPath != "" {
		fmt.Printf("\nResultados por partida em %s\n", csvPath)
	}
	return nil
}
