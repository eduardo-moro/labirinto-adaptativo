package main

import (
	"encoding/binary"
	"io"
	"math"
	"os"
	"os/exec"
	"time"
)

// ───────────────────────────── tons (PulseAudio) ─────────────────────────────
//
// O bell do terminal não tem frequência, então o jogo sintetiza as ondas
// e manda PCM (16 bits, mono) para um processo `pacat` que fica aberto.
//
// O stream é alimentado CONTINUAMENTE (blocos de 10 ms, silêncio quando não
// há som): se ele esvaziar, o PulseAudio só volta a tocar depois de juntar
// um prebuffer maior que um bipe curto — e os bipes saem cortados/alternados.
// Um mixer soma os sons ativos, então sons seguidos se sobrepõem em vez de
// serem descartados. O pipe para o pacat é reduzido (sound_linux.go) para a
// escrita bloquear no ritmo da placa de som, mantendo a latência baixa.

const (
	sampleRate   = 44100
	chunkSamples = sampleRate / 100 // 10 ms
	maxVoices    = 8
)

type Note struct {
	Freq float64 // Hz (0 = pausa)
	Dur  time.Duration
	Vol  float64 // 0..1
}

type Sound []Note

// Sons de cada evento (pré-sintetizados).
var (
	sndError = synth(Sound{{196, 45 * time.Millisecond, 0.30}})                                       // blip grave
	sndGrow  = synth(Sound{{392, 110 * time.Millisecond, 0.45}, {262, 190 * time.Millisecond, 0.45}}) // descida
	sndWin   = synth(Sound{{523, 90 * time.Millisecond, 0.4}, {659, 90 * time.Millisecond, 0.4},
		{784, 90 * time.Millisecond, 0.4}, {1047, 260 * time.Millisecond, 0.45}}) // arpejo Dó maior
)

// sndIdeal: "sonar" que fica mais agudo perto da saída (330 Hz longe → 990 Hz na saída).
func sndIdeal(dist, maxDist int) []float64 {
	t := 1.0
	if maxDist > 0 {
		t = 1 - float64(min(dist, maxDist))/float64(maxDist)
	}
	f := 330 * math.Pow(3, t) // escala exponencial: intervalos musicais iguais
	return synth(Sound{{f, 45 * time.Millisecond, 0.18}})
}

// synth: senoide + um pouco do 2º harmônico (timbre mais "8-bit"),
// com ataque/soltura curtos para não estalar.
func synth(s Sound) []float64 {
	var out []float64
	for _, n := range s {
		total := int(float64(sampleRate) * n.Dur.Seconds())
		attack := sampleRate * 4 / 1000
		release := min(total/2, sampleRate*25/1000)
		for i := 0; i < total; i++ {
			env := 1.0
			if i < attack {
				env = float64(i) / float64(attack)
			} else if i > total-release {
				env = float64(total-i) / float64(release)
			}
			ph := 2 * math.Pi * n.Freq * float64(i) / sampleRate
			v := (math.Sin(ph) + 0.25*math.Sin(2*ph)) / 1.25
			if n.Freq == 0 {
				v = 0
			}
			out = append(out, v*env*n.Vol)
		}
	}
	return out
}

type playReq struct {
	pcm        []float64
	onlyIfIdle bool // descarta se já houver som tocando (usado com o bot)
}

type Audio struct {
	ch  chan playReq
	cmd *exec.Cmd
	w   io.WriteCloser
}

// NewAudio inicia o pacat; devolve nil se não houver PulseAudio.
func NewAudio() *Audio {
	path, err := exec.LookPath("pacat")
	if err != nil {
		return nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	setPipeSize(w, 4096)
	cmd := exec.Command(path, "--playback", "--raw", "--format=s16le",
		"--rate=44100", "--channels=1", "--latency-msec=40",
		"--client-name=labirinto", "--stream-name=efeitos")
	cmd.Stdin = r
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return nil
	}
	r.Close()
	a := &Audio{ch: make(chan playReq, 16), cmd: cmd, w: w}
	go a.mix()
	return a
}

func (a *Audio) mix() {
	var voices [][]float64
	acc := make([]float64, chunkSamples)
	buf := make([]byte, chunkSamples*2)
	for {
		for drained := false; !drained; {
			select {
			case r, ok := <-a.ch:
				if !ok {
					return
				}
				if (r.onlyIfIdle && len(voices) > 0) || len(voices) >= maxVoices {
					continue
				}
				voices = append(voices, r.pcm)
			default:
				drained = true
			}
		}
		clear(acc)
		kept := voices[:0]
		for _, v := range voices {
			n := min(len(v), chunkSamples)
			for i := 0; i < n; i++ {
				acc[i] += v[i]
			}
			if len(v) > n {
				kept = append(kept, v[n:])
			}
		}
		voices = kept
		for i, x := range acc {
			x = max(-1, min(1, x))
			binary.LittleEndian.PutUint16(buf[2*i:], uint16(int16(x*32767)))
		}
		if _, err := a.w.Write(buf); err != nil { // bloqueia no ritmo da placa de som
			return
		}
	}
}

func (a *Audio) Play(pcm []float64, onlyIfIdle bool) {
	if a == nil {
		return
	}
	select {
	case a.ch <- playReq{pcm, onlyIfIdle}:
	default:
	}
}

func (a *Audio) Close() {
	if a == nil {
		return
	}
	close(a.ch)
	a.w.Close()
	done := make(chan struct{})
	go func() { a.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		a.cmd.Process.Kill()
	}
}

// ───────────────────────────── bell (fallback) ─────────────────────────────
//
// Sem PulseAudio, usa o bell do terminal: 1 bipe = cresceu, 3 = saída.
// Screen.Beep() escreve direto no terminal sem trava, então os bipes ficam
// numa fila e só são emitidos pelo loop principal (flushBeeps).

const (
	beepGap   = 170 * time.Millisecond
	beepQueue = 4
)

func (a *App) beep(n int) {
	if !a.Sound {
		return
	}
	t := time.Now()
	if len(a.beeps) > 0 {
		if next := a.beeps[len(a.beeps)-1].Add(beepGap); next.After(t) {
			t = next
		}
	}
	for i := 0; i < n && len(a.beeps) < beepQueue; i++ {
		a.beeps = append(a.beeps, t)
		t = t.Add(beepGap)
	}
}

func (a *App) flushBeeps() {
	if len(a.beeps) > 0 && !a.beeps[0].After(time.Now()) {
		a.S.Beep()
		a.beeps = a.beeps[1:]
		if len(a.beeps) > 0 && a.beeps[0].Before(time.Now().Add(beepGap)) {
			a.beeps[0] = time.Now().Add(beepGap)
		}
	}
}

func (a *App) nextBeep() (time.Duration, bool) {
	if len(a.beeps) == 0 {
		return 0, false
	}
	return max(0, time.Until(a.beeps[0])), true
}

// ───────────────────────────── eventos ─────────────────────────────

type moveEvent int

const (
	evIdeal moveEvent = iota
	evError
	evGrow
	evWin
)

func (a *App) sound(ev moveEvent) {
	if !a.Sound {
		return
	}
	if a.Audio == nil {
		switch ev {
		case evGrow:
			a.beep(1)
		case evWin:
			a.beep(3)
		}
		return
	}
	// Com o bot dirigindo, os sons de passo só tocam se nada estiver soando
	// (senão vira barulho na velocidade alta). No modo manual, toca todos.
	bot := a.Mode == ModeObserve && a.O.Bot
	switch ev {
	case evIdeal:
		g := a.G
		maxD := 0
		ar := g.M.Active
		for r := ar.R0; r < ar.R1; r++ {
			for c := ar.C0; c < ar.C1; c++ {
				maxD = max(maxD, g.Dist[r][c])
			}
		}
		a.Audio.Play(sndIdeal(g.Dist[g.PR][g.PC], maxD), bot)
	case evError:
		a.Audio.Play(sndError, bot)
	case evGrow:
		a.Audio.Play(sndGrow, false)
	case evWin:
		a.Audio.Play(sndWin, false)
	}
}
