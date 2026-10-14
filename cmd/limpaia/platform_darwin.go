//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func isTTYWindowOwner() bool {
	// Duplo clique no Finder abre o Terminal com este processo; Quick Action usa --silencioso.
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func pickFolder() (string, error) {
	out, err := exec.Command("osascript", "-e", `activate`, "-e", `POSIX path of (choose folder with prompt "Escolha a pasta com os JPEGs")`).Output()
	return strings.TrimSpace(string(out)), err
}

func openFolder(p string) { _ = exec.Command("open", p).Start() }

// notify mostra o resultado num diálogo (fecha sozinho em 2 min): notificações
// disparadas pelo osascript costumam não aparecer, e sem Terminal o usuário ficaria sem retorno.
func notify(title, msg string) {
	msg = strings.NewReplacer(`\`, ``, `"`, `'`).Replace(msg)
	script := fmt.Sprintf("activate\ndisplay dialog \"%s\" with title \"%s\" buttons {\"OK\"} default button \"OK\" with icon note giving up after 120", strings.ReplaceAll(msg, "\n", `" & return & "`), title)
	_ = exec.Command("osascript", "-e", script).Run()
}

func servicesDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "Services")
}

const wfName = "Remover metadados de IA.workflow"

// Quick Action (Serviço) do Finder: aparece em botão direito > Ações Rápidas.
const installName = "LimpaIA"

// installDir é a pasta fixa onde o programa se copia ao instalar a Ação Rápida.
func installDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "Application Support", "LimpaIA")
}

func installMenu() error {
	if _, err := installCopy(); err != nil {
		return err
	}
	root := filepath.Join(servicesDir(), wfName, "Contents")
	if err := os.MkdirAll(filepath.Join(root, "Resources"), 0o755); err != nil {
		return err
	}
	info := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>NSServices</key><array><dict>
<key>NSMenuItem</key><dict><key>default</key><string>Remover metadados de IA</string></dict>
<key>NSMessage</key><string>runWorkflowAsService</string>
<key>NSRequiredContext</key><dict><key>NSApplicationIdentifier</key><string>com.apple.finder</string></dict>
<key>NSSendFileTypes</key><array><string>public.folder</string><string>public.jpeg</string></array>
</dict></array></dict></plist>`
	// uma única execução para todos os itens selecionados (um relatório só);
	// "exit 0" evita o alerta do Automator quando há erros (o resumo já é notificado)
	cmd := `"$HOME/Library/Application Support/LimpaIA/LimpaIA" --silencioso "$@"; exit 0`
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>AMApplicationBuild</key><string>523</string><key>AMApplicationVersion</key><string>2.10</string>
<key>AMDocumentVersion</key><string>2</string>
<key>actions</key><array><dict><key>action</key><dict>
<key>AMAccepts</key><dict><key>Container</key><string>List</string><key>Optional</key><false/><key>Types</key><array><string>com.apple.cocoa.path</string></array></dict>
<key>AMActionVersion</key><string>2.0.3</string>
<key>AMApplication</key><array><string>Automator</string></array>
<key>AMParameterProperties</key><dict><key>COMMAND_STRING</key><dict/><key>CheckedForUserDefaultShell</key><dict/><key>inputMethod</key><dict/><key>shell</key><dict/><key>source</key><dict/></dict>
<key>AMProvides</key><dict><key>Container</key><string>List</string><key>Types</key><array><string>com.apple.cocoa.path</string></array></dict>
<key>ActionBundlePath</key><string>/System/Library/Automator/Run Shell Script.action</string>
<key>ActionName</key><string>Run Shell Script</string>
<key>ActionParameters</key><dict>
<key>COMMAND_STRING</key><string>` + cmd + `</string><key>CheckedForUserDefaultShell</key><true/>
<key>inputMethod</key><integer>1</integer><key>shell</key><string>/bin/zsh</string><key>source</key><string></string></dict>
<key>BundleIdentifier</key><string>com.apple.RunShellScript</string>
<key>CFBundleVersion</key><string>2.0.3</string>
<key>Class Name</key><string>RunShellScriptAction</string>
</dict></dict></array>
<key>connectors</key><dict/>
<key>workflowMetaData</key><dict><key>serviceInputTypeIdentifier</key><string>com.apple.Automator.fileSystemObject</string><key>serviceOutputTypeIdentifier</key><string>com.apple.Automator.nothing</string><key>serviceProcessesInput</key><integer>0</integer><key>workflowTypeIdentifier</key><string>com.apple.Automator.servicesMenu</string></dict>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(root, "Info.plist"), []byte(info), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "document.wflow"), []byte(doc), 0o644); err != nil {
		return err
	}
	_ = exec.Command("/System/Library/CoreServices/pbs", "-update").Run()
	fmt.Println("Ative em: Ajustes do Sistema > Teclado > Atalhos de Teclado > Serviços > Arquivos e Pastas, se não aparecer.")
	return nil
}

func uninstallMenu() error {
	removeCopy()
	return os.RemoveAll(filepath.Join(servicesDir(), wfName))
}

func menuInstalled() bool {
	_, err := os.Stat(filepath.Join(servicesDir(), wfName))
	return err == nil
}

func askYesNo(title, msg string) bool {
	msg = strings.NewReplacer(`\`, ``, `"`, `\"`).Replace(msg)
	script := fmt.Sprintf(`button returned of (display dialog "%s" with title "%s" buttons {"Agora não", "Instalar"} default button "Instalar")`, strings.ReplaceAll(msg, "\n", `" & return & "`), title)
	out, err := exec.Command("osascript", "-e", script).Output()
	return err == nil && strings.TrimSpace(string(out)) == "Instalar"
}
