// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// version é gravada no build: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const (
	repo        = "alberthpalhares/limpaia"
	checkEvery  = 7 * 24 * time.Hour // no máximo uma verificação por semana
	maxDownload = 200 << 20
)

// Atualização: no máximo uma vez por semana, ao abrir o app ou usar o menu, o
// LimpaIA pergunta ao GitHub qual é a versão mais recente (nada sobre as fotos
// é enviado). Se houver uma nova, oferece atualizar: baixa o pacote da release,
// confere o SHA-256 e troca o programa no lugar. Desliga com "Não avisar mais"
// ou LIMPAIA_SEM_ATUALIZACAO=1.

func stateDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "LimpaIA")
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// semver devolve [maior, menor, correção] de "vX.Y.Z" (ok=false se não for).
func semver(v string) (n [3]int, ok bool) {
	p := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(p) != 3 {
		return n, false
	}
	for i, s := range p {
		x, err := strconv.Atoi(s)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

func newer(latest, cur string) bool {
	a, ok1 := semver(latest)
	b, ok2 := semver(cur)
	if !ok1 || !ok2 {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

func get(url string, limit int64, timeout time.Duration) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "LimpaIA/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	c := *httpClient
	c.Timeout = timeout
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = errors.New("download grande demais")
	}
	return b, err
}

// assetName é o pacote da release para este sistema.
func assetName() string {
	switch runtime.GOOS {
	case "windows":
		return "LimpaIA.exe"
	case "darwin":
		return "LimpaIA-macOS.zip"
	}
	return ""
}

// maybeUpdate verifica (no máximo uma vez por semana) e, se o usuário aceitar,
// troca o programa pela versão nova. Roda depois do processamento das fotos:
// a próxima execução já usa a versão nova.
func maybeUpdate() {
	if version == "dev" || assetName() == "" || os.Getenv("LIMPAIA_SEM_ATUALIZACAO") != "" {
		return
	}
	dir := stateDir()
	if dir == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "nao_atualizar")); err == nil {
		return
	}
	stamp := filepath.Join(dir, "ultima_verificacao")
	if st, err := os.Stat(stamp); err == nil && time.Since(st.ModTime()) < checkEvery {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(stamp, []byte(time.Now().Format(time.RFC3339)), 0o644)

	api := "https://api.github.com/repos/" + repo + "/releases/latest"
	if u := os.Getenv("LIMPAIA_URL_ATUALIZACAO"); u != "" { // só para testes
		api = u
	}
	b, err := get(api, 1<<20, 5*time.Second)
	if err != nil {
		return // sem internet ou GitHub fora: tenta de novo na semana que vem
	}
	var r release
	if json.Unmarshal(b, &r) != nil || !newer(r.Tag, version) {
		return
	}

	answer := "atualizar"
	if os.Getenv("LIMPAIA_ATUALIZAR_SEM_PERGUNTAR") == "" { // só para testes
		answer = askUpdate("LimpaIA", fmt.Sprintf("Há uma versão nova do LimpaIA: %s (você tem a %s).\n\nAtualizar agora? Leva alguns segundos.", r.Tag, version))
	}
	switch answer {
	case "nunca":
		_ = os.WriteFile(filepath.Join(dir, "nao_atualizar"), []byte("1"), 0o644)
		return
	case "depois":
		return
	}
	fmt.Printf("\nAtualizando para %s...\n", r.Tag)
	exe, err := applyUpdate(r)
	if err != nil {
		fmt.Println("Não foi possível atualizar:", err)
		notify("LimpaIA", "Não foi possível atualizar: "+err.Error())
		return
	}
	// a versão nova renova a cópia do menu e o ícone
	_ = exec.Command(exe, "--atualizado").Run()
	fmt.Printf("LimpaIA atualizado para %s. A próxima vez que você usar já será a versão nova.\n", r.Tag)
	notify("LimpaIA", "LimpaIA atualizado para "+r.Tag+".")
}

// applyUpdate baixa o pacote da release, confere o SHA-256 e substitui o
// programa. Devolve o caminho do executável novo.
func applyUpdate(r release) (string, error) {
	urls := map[string]string{}
	for _, a := range r.Assets {
		urls[a.Name] = a.URL
	}
	pkgURL, sumsURL := urls[assetName()], urls["SHA256SUMS.txt"]
	if pkgURL == "" || sumsURL == "" {
		return "", errors.New("pacote da versão nova não encontrado")
	}
	sums, err := get(sumsURL, 1<<16, 30*time.Second)
	if err != nil {
		return "", err
	}
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && f[1] == assetName() {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return "", errors.New("SHA-256 do pacote não encontrado")
	}
	pkg, err := get(pkgURL, maxDownload, 5*time.Minute)
	if err != nil {
		return "", err
	}
	if h := sha256.Sum256(pkg); hex.EncodeToString(h[:]) != want {
		return "", errors.New("o arquivo baixado não confere (SHA-256 diferente); nada foi alterado")
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	if runtime.GOOS == "darwin" {
		return replaceMac(exe, pkg)
	}
	return exe, replaceFile(exe, pkg)
}

// replaceFile troca um executável, inclusive o que está em uso: no Windows um
// .exe aberto não pode ser apagado, mas pode ser renomeado.
func replaceFile(dst string, data []byte) error {
	tmp := dst + ".novo"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	old := dst + ".old"
	_ = os.Remove(old)
	if err := os.Rename(dst, old); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	_ = os.Remove(old) // no Windows falha enquanto o antigo estiver aberto: limpo na próxima execução
	return nil
}

// cleanupOld apaga a versão anterior deixada por replaceFile.
func cleanupOld() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

// replaceMac extrai o LimpaIA.app do zip e troca o app (quando o programa roda
// de dentro dele) e/ou o executável usado pelo menu.
func replaceMac(exe string, pkg []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		return "", err
	}
	var bin []byte
	for _, f := range zr.File {
		if f.Name == "LimpaIA.app/Contents/MacOS/LimpaIA" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			bin, err = io.ReadAll(io.LimitReader(rc, maxDownload))
			rc.Close()
			if err != nil {
				return "", err
			}
		}
	}
	if bin == nil {
		return "", errors.New("pacote sem o LimpaIA.app")
	}

	// app a atualizar: o deste executável, ou os locais padrão
	var apps []string
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		apps = append(apps, exe[:i+4])
	}
	h, _ := os.UserHomeDir()
	for _, a := range []string{"/Applications/LimpaIA.app", filepath.Join(h, "Applications", "LimpaIA.app")} {
		if st, err := os.Stat(a); err == nil && st.IsDir() && (len(apps) == 0 || apps[0] != a) {
			apps = append(apps, a)
		}
	}
	for _, app := range apps {
		if err := replaceBundle(app, zr); err != nil {
			return "", fmt.Errorf("%s: %w", app, err)
		}
	}
	if len(apps) == 0 || !strings.HasPrefix(exe, apps[0]) {
		// rodando da cópia do menu (Application Support): troca só o executável
		if err := replaceFile(exe, bin); err != nil {
			return "", err
		}
	}
	return exe, nil
}

// replaceBundle extrai o .app do zip ao lado do atual e troca os dois.
func replaceBundle(app string, zr *zip.Reader) error {
	tmp := app + ".novo"
	_ = os.RemoveAll(tmp)
	for _, f := range zr.File {
		rel, ok := strings.CutPrefix(f.Name, "LimpaIA.app/")
		if !ok || rel == "" || strings.Contains(rel, "..") {
			continue
		}
		p := filepath.Join(tmp, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxDownload))
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, data, f.Mode().Perm()|0o400); err != nil {
			return err
		}
	}
	old := app + ".old"
	_ = os.RemoveAll(old)
	if err := os.Rename(app, old); err != nil {
		return err
	}
	if err := os.Rename(tmp, app); err != nil {
		_ = os.Rename(old, app)
		return err
	}
	return os.RemoveAll(old)
}
