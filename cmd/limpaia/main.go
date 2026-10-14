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

const usage = `LimpaIA - remove o indicador de IA (C2PA / Content Credentials) de JPEGs,
sem recomprimir e preservando autoria, copyright, Exif e perfil ICC.

Uso:
  LimpaIA                      abre o seletor de pasta
  LimpaIA <pasta|arquivo>...   processa (ou arraste a pasta sobre o programa)
  LimpaIA --verificar <...>    apenas diagnostica, nao grava nada
  LimpaIA --instalar           adiciona "Remover metadados de IA" ao botao direito
  LimpaIA --desinstalar        remove do botao direito

Saida: subpasta ` + runner.OutDirName + ` ao lado dos originais (que ficam intactos).
`

func main() {
	var inputs []string
	dry, install, uninstall, quiet := false, false, false, false
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
		case "-h", "--help", "/?":
			fmt.Print(usage)
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
		fmt.Println("Erro:", err)
		notify("LimpaIA", "Erro: "+err.Error())
		pause(quiet)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("Nenhum arquivo .jpg/.jpeg encontrado.")
		notify("LimpaIA", "Nenhum arquivo .jpg/.jpeg encontrado.")
		pause(quiet)
		return
	}
	fmt.Printf("Processando %d JPEG(s)%s...\n\n", len(files), map[bool]string{true: " (somente diagnostico)", false: ""}[dry])
	sum := runner.Run(files, dry, os.Stdout)
	fmt.Println("\n" + sum.Text())
	if sum.Report != "" {
		fmt.Println("Relatorio:", sum.Report)
		if runtime.GOOS != "linux" {
			openFolder(sum.OutDirs[0])
		}
	}
	notify("LimpaIA", strings.ReplaceAll(sum.Text(), "\n\n", "\n"))
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
