package runner

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Process é o núcleo sem disco usado pela versão web.
func TestProcess(t *testing.T) {
	ai := jpegAI()
	out, fr := Process("a.jpg", ai)
	if fr.Status != "OK" || len(fr.Found) == 0 || fr.Dims != "20x10" || bytes.Contains(out, []byte("c2pa")) {
		t.Fatalf("com IA: %+v", fr)
	}

	plain := []byte{0xFF, 0xD8}
	plain = append(plain, seg(0xE1, []byte("Exif\x00\x00II*\x00"))...)
	plain = append(plain, seg(0xC0, []byte{8, 0, 10, 0, 20, 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})...)
	plain = append(plain, seg(0xDA, []byte{3, 1, 0, 2, 0x11, 3, 0x11, 0, 63, 0})...)
	plain = append(plain, 0x12, 0x34, 0xFF, 0xD9)
	out, fr = Process("b.jpg", plain)
	if fr.Status != "SEM_IA" || !bytes.Equal(out, plain) {
		t.Fatalf("sem IA: %+v", fr)
	}

	if out, fr = Process("c.jpg", []byte("não é jpeg")); fr.Status != "ERRO" || out != nil || fr.Err == "" {
		t.Fatalf("inválido: %+v", fr)
	}
}

// Só as fotos com indicação de IA vão para a subpasta; sem nenhuma, nada é criado.
func TestOnlyAIWritten(t *testing.T) {
	plain := []byte{0xFF, 0xD8}
	plain = append(plain, seg(0xC0, []byte{8, 0, 10, 0, 20, 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})...)
	plain = append(plain, seg(0xDA, []byte{3, 1, 0, 2, 0x11, 3, 0x11, 0, 63, 0})...)
	plain = append(plain, 0x12, 0x34, 0xFF, 0xD9)

	d := t.TempDir()
	write(t, filepath.Join(d, "limpa.jpg"), plain)
	sum := Run([]string{filepath.Join(d, "limpa.jpg")}, false, io.Discard)
	if _, err := os.Stat(filepath.Join(d, OutDirName)); err == nil || sum.Report != "" {
		t.Fatal("criou a subpasta sem nenhuma foto com IA")
	}
	if !strings.Contains(sum.Text(), "nada foi gravado") {
		t.Fatalf("resumo: %q", sum.Text())
	}

	write(t, filepath.Join(d, "ia.jpg"), jpegAI())
	sum = Run([]string{filepath.Join(d, "ia.jpg"), filepath.Join(d, "limpa.jpg")}, false, io.Discard)
	if _, err := os.Stat(filepath.Join(d, OutDirName, "ia.jpg")); err != nil {
		t.Fatal("foto com IA não foi gravada")
	}
	if _, err := os.Stat(filepath.Join(d, OutDirName, "limpa.jpg")); err == nil {
		t.Fatal("foto sem IA foi copiada")
	}
	if sum.Report == "" || sum.Results[1].Status != "SEM_IA" {
		t.Fatalf("%+v", sum)
	}
}

func TestReportCSVAndText(t *testing.T) {
	s := Summary{Results: []FileResult{{Name: "=cmd.jpg", Status: "OK", Found: []string{"x"}}}}
	csv := string(ReportCSV(s))
	if !strings.HasPrefix(csv, "\xEF\xBB\xBF") || !strings.Contains(csv, "'=cmd.jpg") {
		t.Fatalf("csv: %q", csv)
	}
	txt := ReportText(s, time.Date(2026, 10, 14, 13, 0, 0, 0, time.UTC))
	if !strings.Contains(txt, "14/10/2026 13:00:00") || !strings.Contains(txt, "Continham indicador de IA  1") {
		t.Fatalf("txt: %q", txt)
	}
}
