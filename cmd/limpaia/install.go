package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// installCopy copia o próprio executável para uma pasta fixa do usuário
// (installDir) e devolve o caminho da cópia, que é o que o menu do botão
// direito passa a chamar. Vantagens:
//   - o menu continua funcionando se o arquivo baixado for movido ou apagado;
//   - a cópia feita por este programa não carrega a marca de "baixado da
//     internet" (Mark of the Web no Windows, quarentena no macOS), então o menu
//     não dispara SmartScreen/Gatekeeper a cada uso;
//   - no macOS evita o "App Translocation" (caminho temporário somente leitura).
func installCopy() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	dir := installDir()
	dst := filepath.Join(dir, installName)
	if a, err := os.Stat(exe); err == nil {
		if b, err := os.Stat(dst); err == nil && os.SameFile(a, b) {
			return dst, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := copyFile(exe, dst); err != nil {
		if _, serr := os.Stat(dst); serr == nil {
			return dst, nil // cópia anterior em uso: mantém a existente
		}
		return "", fmt.Errorf("copiando para %s: %w", dir, err)
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err == nil {
		err = out.Close()
	} else {
		out.Close()
	}
	if err == nil {
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// removeCopy apaga a cópia fixa (se estiver em uso, simplesmente permanece).
func removeCopy() {
	_ = os.Remove(filepath.Join(installDir(), installName))
	_ = os.Remove(installDir()) // só remove se a pasta ficou vazia
}
