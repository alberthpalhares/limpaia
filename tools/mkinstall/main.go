// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// mkinstall gera os instaladores de uma linha a partir dos modelos em
// packaging/: preenche repositório, versão e SHA-256 dos pacotes e grava
// install.sh (LF), install.ps1 (só ASCII) e SHA256SUMS.txt.
//
// O install.ps1 precisa ser ASCII: o GitHub serve os arquivos da release sem
// charset, e o Windows PowerShell 5.1 (`irm | iex`) os lê como Latin-1, o que
// estraga os acentos. Por isso cada letra acentuada vira $([char]0x00E3) dentro
// das strings com aspas duplas, e os comentários perdem o acento.
//
// Uso: mkinstall -repo dono/repo -tag v1.0.0 -mac <zip> -win <exe> [-dir packaging] [-out dist]
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkinstall:", err)
		os.Exit(1)
	}
}

func sum(path string) string {
	b, err := os.ReadFile(path)
	check(err)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// fill troca cada marcador {{X}} (todos precisam aparecer) e recusa marcadores desconhecidos.
func fill(name, s string, vals map[string]string) string {
	for k, v := range vals {
		if !strings.Contains(s, "{{"+k+"}}") {
			check(fmt.Errorf("%s: marcador {{%s}} ausente", name, k))
		}
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	if m := regexp.MustCompile(`\{\{\w+\}\}`).FindString(s); m != "" {
		check(fmt.Errorf("%s: marcador %s sem valor", name, m))
	}
	return s
}

var plain = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c",
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "É", "E", "Ê", "E", "Í", "I", "Ó", "O", "Ô", "O", "Õ", "O", "Ú", "U", "Ç", "C",
)

// asciiPS converte o script PowerShell para ASCII sem mudar o texto exibido.
// Fora de comentários, um caractere não ASCII só é aceito dentro de string com
// aspas duplas ou here-string @" "@ (onde $(...) é expandido).
func asciiPS(s string) string {
	var out []string
	here := false
	for n, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case here && strings.HasPrefix(t, `"@`):
			here = false
		case !here && strings.HasSuffix(t, `@"`):
			here = true
			out = append(out, line)
			continue
		}
		if !here && strings.HasPrefix(t, "#") {
			line = plain.Replace(line)
		}
		var b strings.Builder
		sq, dq := false, false
		for i, r := range line {
			if !here {
				switch {
				case r == '\'' && !dq:
					sq = !sq
				case r == '"' && !sq:
					dq = !dq
				case r == '#' && !sq && !dq: // comentário no fim da linha
					b.WriteString(plain.Replace(line[i:]))
				}
				if r == '#' && !sq && !dq {
					break
				}
			}
			if r < 0x80 {
				b.WriteRune(r)
				continue
			}
			if !here && !dq {
				check(fmt.Errorf("install.ps1 linha %d: %q fora de string com aspas duplas", n+1, r))
			}
			if r > 0xFFFF {
				check(fmt.Errorf("install.ps1 linha %d: caractere %q não suportado", n+1, r))
			}
			fmt.Fprintf(&b, "$([char]0x%04X)", r)
		}
		out = append(out, b.String())
	}
	return strings.Join(out, "\r\n")
}

func main() {
	repo := flag.String("repo", "", "repositório no GitHub (dono/nome)")
	tag := flag.String("tag", "", "versão da release (ex.: v1.0.0)")
	mac := flag.String("mac", "", "LimpaIA-macOS.zip")
	win := flag.String("win", "", "LimpaIA.exe")
	dir := flag.String("dir", "packaging", "pasta com install.sh e install.ps1")
	out := flag.String("out", "dist", "pasta de saída")
	flag.Parse()
	if *repo == "" || *tag == "" || *mac == "" || *win == "" {
		flag.Usage()
		os.Exit(2)
	}
	if !regexp.MustCompile(`^[\w.-]+/[\w.-]+$`).MatchString(*repo) || !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(*tag) {
		check(fmt.Errorf("repo %q ou tag %q inválidos (esperado dono/nome e vX.Y.Z)", *repo, *tag))
	}
	shaMac, shaWin := sum(*mac), sum(*win)

	read := func(n string) string {
		b, err := os.ReadFile(filepath.Join(*dir, n))
		check(err)
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	sh := fill("install.sh", read("install.sh"), map[string]string{"REPO": *repo, "TAG": *tag, "SHA256_MAC": shaMac})
	ps := fill("install.ps1", read("install.ps1"), map[string]string{"REPO": *repo, "TAG": *tag, "SHA256_WIN": strings.ToUpper(shaWin)})
	ps = asciiPS(ps)
	for i, r := range ps {
		if r >= 0x80 {
			check(fmt.Errorf("install.ps1: caractere %q sem conversão para ASCII (posição %d)", r, i))
		}
	}

	check(os.WriteFile(filepath.Join(*out, "install.sh"), []byte(sh), 0o755))
	check(os.WriteFile(filepath.Join(*out, "install.ps1"), []byte(ps), 0o644))
	sums := fmt.Sprintf("%s  %s\n%s  %s\n", shaMac, filepath.Base(*mac), shaWin, filepath.Base(*win))
	check(os.WriteFile(filepath.Join(*out, "SHA256SUMS.txt"), []byte(sums), 0o644))
	fmt.Printf("%s: install.sh, install.ps1 e SHA256SUMS.txt para %s %s\n", *out, *repo, *tag)
}
