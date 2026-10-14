// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Seleção de várias fotos no Explorer: o Windows abre um processo por arquivo
// selecionado. Cada processo deixa o seu caminho numa fila; o primeiro a pegar
// a trava vira "líder", junta tudo o que chegar em seguida e processa num lote
// só (um relatório, uma janela). Os demais saem assim que o líder recolhe o
// pedido deles. Pedidos que chegarem depois do lote viram um lote próprio.

const (
	gatherQuiet = 700 * time.Millisecond // sem pedidos novos por este tempo: fecha o lote
	gatherMax   = 10 * time.Second       // teto da espera do líder
	waitMax     = 10 * time.Minute       // teto da espera de quem não é líder
	staleLock   = 10 * time.Minute
)

func queueDir() string {
	if d := installDir(); d != "" {
		return filepath.Join(d, "fila")
	}
	return filepath.Join(os.TempDir(), "LimpaIA-fila")
}

func randID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// enqueue grava os caminhos (absolutos) como um pedido na fila.
func enqueue(dir string, paths []string) (string, error) {
	var abs []string
	for _, p := range paths {
		if a, err := filepath.Abs(p); err == nil {
			p = a
		}
		abs = append(abs, p)
	}
	name := fmt.Sprintf("%020d-%s.pedido", time.Now().UnixNano(), randID())
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, []byte(strings.Join(abs, "\n")), 0o644); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	return dst, os.Rename(tmp, dst)
}

// drain recolhe (e apaga) os pedidos completos da fila.
func drain(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".pedido") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // ordem de chegada
	var out []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		b, err := os.ReadFile(p)
		if err != nil || os.Remove(p) != nil {
			continue // outro líder pegou primeiro
		}
		for _, l := range strings.Split(string(b), "\n") {
			if l != "" {
				out = append(out, l)
			}
		}
	}
	return out
}

func tryLock(lock string) bool {
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if st, e := os.Stat(lock); e == nil && time.Since(st.ModTime()) > staleLock {
			_ = os.Remove(lock) // líder anterior travou ou foi encerrado
		}
		return false
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	f.Close()
	return true
}

// gatherSelection devolve o lote a processar (este processo é o líder) ou nil
// (o pedido foi entregue a outro processo, que vai processá-lo). release deve
// ser chamada pelo líder ao terminar.
func gatherSelection(paths []string) (batch []string, release func()) {
	none := func() {}
	dir := queueDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return paths, none // sem fila: processa sozinho
	}
	mine, err := enqueue(dir, paths)
	if err != nil {
		return paths, none
	}
	lock := filepath.Join(dir, "lider.trava")
	start := time.Now()
	for !tryLock(lock) {
		if _, err := os.Stat(mine); os.IsNotExist(err) {
			return nil, none // o líder recolheu o pedido
		}
		if time.Since(start) > waitMax {
			if drained := drainOne(mine); drained != nil {
				return drained, none
			}
			return nil, none
		}
		time.Sleep(50 * time.Millisecond)
	}
	release = func() { _ = os.Remove(lock) }

	seen := map[string]bool{}
	add := func(ps []string) bool {
		n := false
		for _, p := range ps {
			if k := strings.ToLower(p); !seen[k] {
				seen[k] = true
				batch = append(batch, p)
				n = true
			}
		}
		return n
	}
	last := time.Now()
	for time.Since(start) < gatherMax {
		if add(drain(dir)) {
			last = time.Now()
		}
		if time.Since(last) >= gatherQuiet {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(mine); err == nil { // o próprio pedido ainda não saiu da fila
		add(drainOne(mine))
	}
	return batch, release
}

// drainOne recolhe só o pedido indicado (se ainda estiver na fila).
func drainOne(p string) []string {
	b, err := os.ReadFile(p)
	if err != nil || os.Remove(p) != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}
