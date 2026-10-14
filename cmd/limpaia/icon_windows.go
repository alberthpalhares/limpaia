// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	_ "embed"
	"os"
	"path/filepath"
)

// LimpaIA.ico é gerado por tools/mkicon (cópia de assets/icon/LimpaIA.ico).
//
//go:embed LimpaIA.ico
var iconICO []byte

const iconName = "LimpaIA.ico"

// installIcon grava o ícone ao lado da cópia instalada e devolve o caminho
// para o valor "Icon" do menu (se falhar, usa o próprio executável).
func installIcon(exe string) string {
	p := filepath.Join(filepath.Dir(exe), iconName)
	if err := os.WriteFile(p, iconICO, 0o644); err != nil {
		return exe
	}
	return p
}

func removeIcon() { _ = os.Remove(filepath.Join(installDir(), iconName)) }
