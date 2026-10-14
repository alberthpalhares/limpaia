// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"limpaia/internal/runner"
)

const usage = `Remove o indicador de IA (C2PA / Content Credentials) de fotos JPEG, PNG e
WebP, sem recomprimir e preservando autoria, copyright, Exif e perfil de cor.

Uso:
  LimpaIA                      abre o seletor de pasta
  LimpaIA <pasta|foto>...      processa (ou arraste a pasta/fotos sobre o programa)
  LimpaIA --verificar <...>    apenas diagnostica, nao grava nada
  LimpaIA --instalar           adiciona "Remover metadados de IA" ao botao direito
  LimpaIA --desinstalar        remove do botao direito
  LimpaIA --versao             mostra a versao

Saida: subpasta ` + runner.OutDirName + ` ao lado dos originais (que ficam intactos),
so com as fotos que tinham indicacao de IA, e um relatorio.

`

func main() {
	cleanupOld()
	var inputs []string
	dry, install, uninstall, quiet, menu, selection, updated := false, false, false, false, false, false, false
	for _, a := range os.Args[1:] {
		switch strings.ToLower(a) {
		case "--verificar", "--diagnostico":
			dry = true
		case "--instalar":
			install = true
		case "--desinstalar":
			uninstall = true
		case "--silencioso":
			quiet = true
		case "--menu": // chamado pelo botão direito
			menu = true
		case "--selecao": // uma das fotos selecionadas no Explorer (um processo por foto)
			selection = true
		case "--atualizado": // reaberto pela versão anterior depois de atualizar
			updated = true
		case "--versao", "--version":
			fmt.Println("LimpaIA", version)
			return
		case "-h", "--help", "/?":
			fmt.Printf("LimpaIA %s\n%s\n\n%s", version, runner.Credit, usage)
			fmt.Println(runner.Notice)
			return
		default:
			if runtime.GOOS == "windows" {
				// "%V" numa raiz de drive vira "E:\" e o Windows entrega E:" (aspas literais)
				a = strings.TrimSuffix(a, `"`)
				if len(a) == 2 && a[1] == ':' {
					a += `\`
				}
			}
			inputs = append(inputs, a)
		}
	}
	interactive := len(inputs) == 0 && !install && !uninstall

	if updated { // chamado pela versão anterior logo depois de atualizar
		if menuInstalled() {
			_ = installMenu() // renova a cópia do menu, o ícone e as chaves com a versão nova
		}
		return
	}

	var release func()
	if selection && !install && !uninstall {
		var batch []string
		batch, release = gatherSelection(inputs)
		if batch == nil {
			return // outro processo da mesma seleção vai processar esta foto
		}
		inputs = batch
	}
	if release == nil {
		release = func() {}
	}

	switch {
	case install:
		exitIf(installMenu())
		fmt.Println("Menu de contexto instalado.")
		pause(quiet)
		return
	case uninstall:
		exitIf(uninstallMenu())
		fmt.Println("Menu de contexto removido.")
		pause(quiet)
		return
	}

	if interactive {
		if menuInstalled() {
			_, _ = installCopy() // mantém a cópia usada pelo menu na versão mais recente
		}
		offerMenuInstall()
		dir, err := pickFolder()
		if err != nil || dir == "" {
			fmt.Println("Nenhuma pasta selecionada.")
			pause(quiet)
			return
		}
		inputs = []string{dir}
	}

	files, err := runner.Collect(inputs)
	if err != nil {
		release()
		fmt.Println("Erro:", err)
		notify("LimpaIA", "Erro: "+err.Error())
		pause(quiet)
		os.Exit(1)
	}
	if len(files) == 0 {
		release()
		fmt.Println("Nenhuma foto .jpg, .jpeg, .png ou .webp encontrada.")
		notify("LimpaIA", "Nenhuma foto .jpg, .jpeg, .png ou .webp encontrada.")
		pause(quiet)
		return
	}
	fmt.Printf("LimpaIA %s · por Alberth Palhares\n", version)
	fmt.Printf("Processando %d foto(s)%s...\n\n", len(files), map[bool]string{true: " (somente diagnostico)", false: ""}[dry])
	sum := runner.Run(files, dry, os.Stdout)
	release()
	fmt.Println("\n" + sum.Text())
	if sum.Report != "" {
		fmt.Println("Relatorio:", sum.Report)
		if runtime.GOOS != "linux" {
			openFolder(sum.OutDirs[0])
		}
	}
	notify("LimpaIA", strings.ReplaceAll(sum.Text(), "\n\n", "\n"))
	if (interactive || menu) && !dry {
		maybeUpdate() // no máximo uma vez por semana; só pergunta se houver versão nova
	}
	pause(quiet)
	if sum.Errors() > 0 || sum.Persist() > 0 {
		os.Exit(2)
	}
}

func exitIf(err error) {
	if err != nil {
		fmt.Println("Erro:", err)
		os.Exit(1)
	}
}

// pause evita que a janela feche antes de o usuario ler o resultado.
func pause(quiet bool) {
	if quiet || !isTTYWindowOwner() {
		return
	}
	fmt.Print("\nPressione Enter para fechar...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// offerMenuInstall pergunta, no duplo clique, se deseja instalar o menu do botão direito.
// "Não" é lembrado (apague a pasta LimpaIA em UserConfigDir para perguntar de novo).
func offerMenuInstall() {
	if menuInstalled() {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	marker := filepath.Join(dir, "LimpaIA", "nao_perguntar")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	if !askYesNo("LimpaIA", "Deseja adicionar \"Remover metadados de IA\" ao menu do botão direito?") {
		_ = os.MkdirAll(filepath.Dir(marker), 0o755)
		_ = os.WriteFile(marker, []byte("1"), 0o644)
		return
	}
	if err := installMenu(); err != nil {
		fmt.Println("Não foi possível instalar o menu:", err)
		return
	}
	fmt.Println("Menu do botão direito instalado.")
}
