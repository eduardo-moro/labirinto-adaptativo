package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// setPipeSize reduz o buffer do pipe (padrão 64 KB ≈ 0,7 s de áudio) para
// que a escrita bloqueie no ritmo da placa de som e a latência fique baixa.
func setPipeSize(f *os.File, n int) {
	unix.FcntlInt(f.Fd(), unix.F_SETPIPE_SZ, n)
}
