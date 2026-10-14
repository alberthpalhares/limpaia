//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// Tudo aqui usa a API nativa do Windows (user32/shell32/advapi32). Evita abrir
// powershell.exe oculto e reg.exe, comportamento que antivírus e SmartScreen
// costumam tratar como suspeito em executáveis sem assinatura.

const menuText = "Remover metadados de IA"

var (
	k32    = syscall.NewLazyDLL("kernel32.dll")
	user32 = syscall.NewLazyDLL("user32.dll")
	shell  = syscall.NewLazyDLL("shell32.dll")
	ole32  = syscall.NewLazyDLL("ole32.dll")
)

// isTTYWindowOwner: true quando este processo é o único no console
// (duplo clique ou menu de contexto), ou seja, a janela fecharia ao terminar.
func isTTYWindowOwner() bool {
	var pids [4]uint32
	n, _, _ := k32.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&pids[0])), 4)
	return n <= 1
}

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// installDir é a pasta fixa onde o programa se copia ao instalar o menu.
func installDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "LimpaIA")
	}
	d, _ := os.UserCacheDir()
	return filepath.Join(d, "LimpaIA")
}

const installName = "LimpaIA.exe"

func pickFolder() (string, error) {
	// o seletor de pastas é COM: precisa de thread própria e CoInitialize
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const coinitApartment = 0x2
	ole32.NewProc("CoInitializeEx").Call(0, coinitApartment)
	defer ole32.NewProc("CoUninitialize").Call()

	type browseInfo struct {
		owner       uintptr
		root        uintptr
		displayName *uint16
		title       *uint16
		flags       uint32
		fn          uintptr
		lParam      uintptr
		image       int32
	}
	var disp [260]uint16
	bi := browseInfo{
		displayName: &disp[0],
		title:       u16("Escolha a pasta com os JPEGs"),
		flags:       0x1 | 0x40, // BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE
	}
	pidl, _, _ := shell.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil // cancelado
	}
	defer ole32.NewProc("CoTaskMemFree").Call(pidl)
	var path [1024]uint16
	if ok, _, _ := shell.NewProc("SHGetPathFromIDListEx").Call(pidl, uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)), 0); ok == 0 {
		return "", fmt.Errorf("pasta inválida")
	}
	return syscall.UTF16ToString(path[:]), nil
}

func openFolder(p string) { _ = exec.Command("explorer.exe", p).Start() }

func notify(title, msg string) {}

func askYesNo(title, msg string) bool {
	const yesNo, question, foreground, topmost = 0x4, 0x20, 0x10000, 0x40000
	r, _, _ := user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(u16(msg))), uintptr(unsafe.Pointer(u16(title))), yesNo|question|foreground|topmost)
	return r == 6 // IDYES
}

// ---------- registro (HKCU, sem admin) ----------

const (
	hkcu     = 0x80000001 // HKEY_CURRENT_USER
	keyWrite = 0x20006
	keyRead  = 0x20019
	regSz    = 1
)

var advapi32 = syscall.NewLazyDLL("advapi32.dll")

func regErr(path string, r uintptr) error {
	return fmt.Errorf("registro %s: %w", path, syscall.Errno(r))
}

func regSet(path, name, value string) error {
	var k syscall.Handle
	if r, _, _ := advapi32.NewProc("RegCreateKeyExW").Call(hkcu, uintptr(unsafe.Pointer(u16(path))), 0, 0, 0, keyWrite, 0, uintptr(unsafe.Pointer(&k)), 0); r != 0 {
		return regErr(path, r)
	}
	defer syscall.RegCloseKey(k)
	v := syscall.StringToUTF16(value)
	var n uintptr // nome vazio = valor padrão
	if name != "" {
		n = uintptr(unsafe.Pointer(u16(name)))
	}
	if r, _, _ := advapi32.NewProc("RegSetValueExW").Call(uintptr(k), n, 0, regSz, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2)); r != 0 {
		return regErr(path, r)
	}
	return nil
}

func regDelete(path string) {
	_, _, _ = advapi32.NewProc("RegDeleteKeyW").Call(hkcu, uintptr(unsafe.Pointer(u16(path))))
}

func regExists(path string) bool {
	var k syscall.Handle
	if r, _, _ := advapi32.NewProc("RegOpenKeyExW").Call(hkcu, uintptr(unsafe.Pointer(u16(path))), 0, keyRead, uintptr(unsafe.Pointer(&k))); r != 0 {
		return false
	}
	syscall.RegCloseKey(k)
	return true
}

var menuKeys = []struct{ key, arg string }{
	{`Software\Classes\Directory\shell\LimpaIA`, "%1"},
	{`Software\Classes\Directory\Background\shell\LimpaIA`, "%V"},
}

func installMenu() error {
	exe, err := installCopy()
	if err != nil {
		return err
	}
	for _, k := range menuKeys {
		if err := regSet(k.key, "", menuText); err != nil {
			return err
		}
		if err := regSet(k.key, "Icon", exe); err != nil {
			return err
		}
		if err := regSet(k.key+`\command`, "", fmt.Sprintf(`"%s" "%s"`, exe, k.arg)); err != nil {
			return err
		}
	}
	return nil
}

// verbos por arquivo das versões anteriores (removidos: um processo por foto selecionada)
var legacyKeys = []string{
	`Software\Classes\SystemFileAssociations\.jpg\shell\LimpaIA`,
	`Software\Classes\SystemFileAssociations\.jpeg\shell\LimpaIA`,
}

func uninstallMenu() error {
	for _, k := range legacyKeys {
		regDelete(k + `\command`)
		regDelete(k)
	}
	for _, k := range menuKeys {
		regDelete(k.key + `\command`)
		regDelete(k.key)
	}
	removeCopy()
	return nil
}

func menuInstalled() bool { return regExists(menuKeys[0].key) }
