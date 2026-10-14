package runner

// Testes de QA do runner. Testes "Regress_" reproduzem defeitos achados na
// revisão de QA e já corrigidos.

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func seg(m byte, payload []byte) []byte {
	l := len(payload) + 2
	return append([]byte{0xFF, m, byte(l >> 8), byte(l)}, payload...)
}

// jpegAI devolve um JPEG mínimo com APP11/JUMBF (C2PA).
func jpegAI() []byte {
	b := []byte{0xFF, 0xD8}
	b = append(b, seg(0xE1, []byte("Exif\x00\x00II*\x00"))...)
	b = append(b, seg(0xEB, []byte{'J', 'P', 0, 1, 0, 0, 0, 1, 0, 0, 0, 16, 'j', 'u', 'm', 'b', 'c', '2', 'p', 'a'})...)
	b = append(b, seg(0xC0, []byte{8, 0, 10, 0, 20, 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})...)
	b = append(b, seg(0xDA, []byte{3, 1, 0, 2, 0x11, 3, 0x11, 0, 63, 0})...)
	return append(b, 0x12, 0x34, 0xFF, 0xD9)
}

func write(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func hash(t *testing.T, p string) [32]byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func TestQA_CollectVariants(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"A.JPG", "b.jpeg", "c Ção.Jpg", "._d.jpg", "e.png", "f.jpg.tmp", "g.WEBP", "h.gif"} {
		write(t, filepath.Join(d, n), jpegAI())
	}
	os.Mkdir(filepath.Join(d, "sub.jpg"), 0o755)
	got, err := Collect([]string{d, filepath.Join(d, "A.JPG")})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, g := range got {
		names = append(names, filepath.Base(g))
	}
	if strings.Join(names, ",") != "A.JPG,b.jpeg,c Ção.Jpg,e.png,g.WEBP" {
		t.Fatalf("coletados: %v", names)
	}
}

func TestQA_RunOriginalsUntouchedAndOutput(t *testing.T) {
	d := t.TempDir()
	src := filepath.Join(d, "foto ação.jpg")
	write(t, src, jpegAI())
	os.Chmod(src, 0o444)
	h := hash(t, src)
	sum := Run([]string{src}, false, io.Discard)
	if sum.Errors() != 0 || sum.WithAI() != 1 || sum.Persist() != 0 {
		t.Fatalf("%+v", sum.Results)
	}
	if hash(t, src) != h {
		t.Fatal("original alterado")
	}
	out, err := os.ReadFile(filepath.Join(d, OutDirName, "foto ação.jpg"))
	if err != nil || bytes.Contains(out, []byte("jumb")) {
		t.Fatalf("saída: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d, OutDirName, "foto ação.jpg.tmp")); err == nil {
		t.Fatal(".tmp órfão")
	}
	// dry-run não grava nada
	d2 := t.TempDir()
	write(t, filepath.Join(d2, "x.jpg"), jpegAI())
	Run([]string{filepath.Join(d2, "x.jpg")}, true, io.Discard)
	if _, err := os.Stat(filepath.Join(d2, OutDirName)); err == nil {
		t.Fatal("--verificar criou a pasta de saída")
	}
}

// writeAtomic (runner.go:94-95) abre "<dst>.tmp" com O_TRUNC seguindo
// hardlink/symlink pré-existente: o alvo do link é sobrescrito.
func TestQA_Regress_TmpLinkOverwritesOutsideFile(t *testing.T) {
	for _, kind := range []string{"hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			d := t.TempDir()
			victim := filepath.Join(t.TempDir(), "importante.txt")
			write(t, victim, []byte("conteudo do usuario"))
			write(t, filepath.Join(d, "a.jpg"), jpegAI())
			os.Mkdir(filepath.Join(d, OutDirName), 0o755)
			tmp := filepath.Join(d, OutDirName, "a.jpg.tmp")
			var err error
			if kind == "hardlink" {
				err = os.Link(victim, tmp)
			} else {
				err = os.Symlink(victim, tmp)
			}
			if err != nil {
				t.Skipf("não foi possível criar %s: %v", kind, err)
			}
			Run([]string{filepath.Join(d, "a.jpg")}, false, io.Discard)
			if b, _ := os.ReadFile(victim); string(b) != "conteudo do usuario" {
				t.Fatalf("arquivo fora da pasta de saída foi sobrescrito (%d bytes, começa com % X)", len(b), b[:4])
			}
		})
	}
}

// Saída anterior somente leitura: no Windows o rename falha e o .tmp fica órfão.
func TestQA_Regress_ReadOnlyOutputLeavesTmp(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("comportamento específico do Windows")
	}
	d := t.TempDir()
	write(t, filepath.Join(d, "a.jpg"), jpegAI())
	os.Mkdir(filepath.Join(d, OutDirName), 0o755)
	old := filepath.Join(d, OutDirName, "a.jpg")
	write(t, old, []byte("antigo"))
	os.Chmod(old, 0o444)
	defer os.Chmod(old, 0o644)
	sum := Run([]string{filepath.Join(d, "a.jpg")}, false, io.Discard)
	_, err := os.Stat(old + ".tmp")
	if sum.Errors() > 0 || err == nil {
		t.Fatalf("erros=%d tmp_orfao=%v (%s)", sum.Errors(), err == nil, sum.Results[0].Err)
	}
}

// Um único arquivo malicioso derruba o lote inteiro (sem recover); o relatório não é gravado.
func TestQA_Regress_MaliciousFileAbortsBatch(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.jpg"), jpegAI())
	// >512 bytes: os.ReadFile dá cap = tamanho+1, então o slice do GUID estoura
	bad := append([]byte{0xFF, 0xD8}, seg(0xFE, bytes.Repeat([]byte("x"), 600))...)
	bad = append(bad, seg(0xE1, []byte("http://ns.adobe.com/xmp/extension/\x00ABC"))...)
	write(t, filepath.Join(d, "b.jpg"), append(bad, 0xFF, 0xDA, 0, 8))
	files, _ := Collect([]string{d})
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("panic no lote: %v", p)
		}
	}()
	sum := Run(files, false, io.Discard)
	if sum.Errors() != 1 {
		t.Fatalf("esperado 1 erro: %+v", sum.Results)
	}
}

// Duas execuções no mesmo segundo (ex.: vários .jpg selecionados no Explorer
// abrem um processo por arquivo) gravam o mesmo RELATORIO_<ts>: o último sobrescreve.
func TestQA_Regress_ReportOverwrittenSameSecond(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.jpg"), jpegAI())
	write(t, filepath.Join(d, "b.jpg"), jpegAI())
	for time.Now().Nanosecond() > 500e6 { // começa no início de um segundo
		time.Sleep(10 * time.Millisecond)
	}
	r1 := Run([]string{filepath.Join(d, "a.jpg")}, false, io.Discard).Report
	r2 := Run([]string{filepath.Join(d, "b.jpg")}, false, io.Discard).Report
	if r1 == r2 {
		b, _ := os.ReadFile(r1)
		if !bytes.Contains(b, []byte("a.jpg")) {
			t.Fatalf("relatório %s sobrescrito: a.jpg não consta mais", filepath.Base(r1))
		}
	}
}

// Symlink para arquivo especial (ex.: /dev/zero) entra na lista e o
// os.ReadFile nunca termina (DoS de memória). Só Unix/macOS.
func TestQA_Regress_CollectFollowsSpecialFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sem /dev/zero no Windows")
	}
	d := t.TempDir()
	if err := os.Symlink("/dev/zero", filepath.Join(d, "z.jpg")); err != nil {
		t.Skip(err)
	}
	got, _ := Collect([]string{d})
	if len(got) != 0 {
		t.Fatalf("Collect aceitou arquivo não regular: %v (Run faria ReadFile infinito)", got)
	}
}
