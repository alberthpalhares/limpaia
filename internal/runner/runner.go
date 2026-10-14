// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// Package runner processa pastas/arquivos de imagem (JPEG, PNG, WebP), valida e
// gera relatório. Só as fotos com indicação de IA são gravadas na subpasta.
package runner

import (
	"bytes"
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"limpaia/internal/jpegclean"
	"limpaia/internal/pngclean"
	"limpaia/internal/webpclean"
)

const OutDirName = "SEM_METADATA_IA"

// Credit aparece no relatório e nas telas de ajuda.
const Credit = "LimpaIA, por Alberth Palhares · https://github.com/alberthpalhares/limpaia"

// Notice é o lembrete de uso consciente (relatório, ajuda, página web).
const Notice = "Uso consciente: o LimpaIA protege fotos autorais, captadas por câmera, que receberam o rótulo " +
	"\"Feito com IA\" por causa de ajustes finos de edição. Não use para esconder que uma imagem foi gerada por IA, " +
	"enganar o público ou remover a marcação de imagens de outras pessoas. Quem publica é responsável pelo uso."

type FileResult struct {
	Name      string
	Source    string
	Found     []string
	Removed   []string
	Remaining []string
	Status    string // OK | SEM_IA | ERRO | DIAGNOSTICO
	Err       string
	Dims      string
}

type Summary struct {
	Results []FileResult
	OutDirs []string
	Report  string
}

func (s Summary) count(f func(FileResult) bool) (n int) {
	for _, r := range s.Results {
		if f(r) {
			n++
		}
	}
	return
}

func (s Summary) Total() int  { return len(s.Results) }
func (s Summary) WithAI() int { return s.count(func(r FileResult) bool { return len(r.Found) > 0 }) }
func (s Summary) Errors() int { return s.count(func(r FileResult) bool { return r.Status == "ERRO" }) }
func (s Summary) Persist() int {
	return s.count(func(r FileResult) bool { return len(r.Remaining) > 0 })
}

func isImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

// codec reúne as funções de um formato.
type codec struct {
	name      string
	clean     func([]byte) ([]byte, jpegclean.Info, error)
	dims      func([]byte) (int, int, error)
	preserved func(orig, clean []byte) error
}

var (
	cJPEG = codec{"JPEG", jpegclean.Clean, jpegclean.Dimensions, jpegclean.PreservedEqual}
	cPNG  = codec{"PNG", pngclean.Clean, pngclean.Dimensions, pngclean.PreservedEqual}
	cWebP = codec{"WebP", webpclean.Clean, webpclean.Dimensions, webpclean.PreservedEqual}
)

// detect escolhe o formato pela assinatura do arquivo (não pela extensão).
func detect(data []byte) (codec, error) {
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8:
		return cJPEG, nil
	case pngclean.IsPNG(data):
		return cPNG, nil
	case webpclean.IsWebP(data):
		return cWebP, nil
	}
	return codec{}, fmt.Errorf("formato não suportado (esperado JPEG, PNG ou WebP)")
}

// Collect expande pastas (não recursivo) e arquivos em uma lista de JPEGs.
func Collect(inputs []string) ([]string, error) {
	var files []string
	seen := map[string]bool{}
	add := func(p string) {
		if k := strings.ToLower(p); !seen[k] {
			seen[k] = true
			files = append(files, p)
		}
	}
	for _, in := range inputs {
		st, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if isImage(in) && st.Mode().IsRegular() {
				add(in)
			}
			continue
		}
		ents, err := os.ReadDir(in)
		if err != nil {
			return nil, err
		}
		for _, e := range ents {
			if e.IsDir() || !isImage(e.Name()) || strings.HasPrefix(e.Name(), "._") {
				continue
			}
			p := filepath.Join(in, e.Name())
			// segue links simbólicos, mas só aceita arquivo comum (nunca /dev/zero, FIFO...)
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				add(p)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// writeAtomic grava num arquivo temporário novo (O_EXCL: nunca segue link
// pré-existente) e troca pelo destino; em falha, o temporário é apagado.
func writeAtomic(dst string, data []byte) error {
	var f *os.File
	var tmp string
	for i := 0; ; i++ {
		var b [4]byte
		_, _ = rand.Read(b[:])
		tmp = fmt.Sprintf("%s.%x.tmp", dst, b)
		var err error
		if f, err = os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); err == nil {
			break
		} else if !os.IsExist(err) || i > 8 {
			return err
		}
	}
	_, err := f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		// saída anterior somente leitura impediria o rename no Windows
		if st, e := os.Lstat(dst); e == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o200 == 0 {
			_ = os.Chmod(dst, 0o644)
		}
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

const maxFileSize = 1 << 30 // 1 GiB

// readLimited lê o arquivo inteiro, recusando os absurdamente grandes.
func readLimited(p string) ([]byte, error) {
	if st, err := os.Stat(p); err == nil && st.Size() > maxFileSize {
		return nil, fmt.Errorf("arquivo muito grande (%d MiB)", st.Size()>>20)
	}
	return os.ReadFile(p)
}

// csvSafe impede que o Excel trate texto começando com = + - @ como fórmula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// clean executa a limpeza de um arquivo sem deixar uma imagem malformada derrubar o lote.
func clean(c codec, data []byte) (out []byte, info jpegclean.Info, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("arquivo malformado (%v)", r)
		}
	}()
	return c.clean(data)
}

// Process limpa e valida uma imagem já lida, sem tocar no disco (usado também
// pela versão web). Status: OK, SEM_IA ou ERRO; em ERRO, out é nil.
func Process(name string, data []byte) ([]byte, FileResult) {
	fr := FileResult{Name: name}
	c, err := detect(data)
	if err != nil {
		fr.Status, fr.Err = "ERRO", err.Error()
		return nil, fr
	}
	out, info, err := clean(c, data)
	if err == nil {
		err = verify(c, data, out, &fr)
	}
	fr.Found, fr.Removed, fr.Remaining = info.Found, info.Removed, info.Remaining
	switch {
	case err != nil:
		fr.Status, fr.Err = "ERRO", err.Error()
		return nil, fr
	case len(info.Found) == 0:
		fr.Status = "SEM_IA"
	default:
		fr.Status = "OK"
	}
	return out, fr
}

// Run processa os arquivos. Se dryRun, só diagnostica. log recebe progresso.
func Run(files []string, dryRun bool, log io.Writer) Summary {
	var sum Summary
	outDirs := map[string]bool{}
	for i, src := range files {
		fr := FileResult{Name: filepath.Base(src), Source: src}
		fmt.Fprintf(log, "[%d/%d] %s ... ", i+1, len(files), fr.Name)
		data, err := readLimited(src)
		if err != nil {
			fr.Status, fr.Err = "ERRO", err.Error()
			fmt.Fprintln(log, "ERRO:", err)
			sum.Results = append(sum.Results, fr)
			continue
		}
		out, r := Process(fr.Name, data)
		r.Source = src
		fr = r
		switch {
		case fr.Status == "ERRO":
		case dryRun:
			fr.Status = "DIAGNOSTICO"
			fr.Remaining = nil
		case fr.Status == "SEM_IA":
			// sem indicação de IA: nada a limpar, a foto não é copiada
		default:
			dir := filepath.Join(filepath.Dir(src), OutDirName)
			if err = os.MkdirAll(dir, 0o755); err == nil {
				err = writeAtomic(filepath.Join(dir, fr.Name), out)
			}
			if err != nil {
				fr.Status, fr.Err = "ERRO", err.Error()
			} else {
				outDirs[dir] = true
			}
		}
		switch {
		case fr.Status == "ERRO":
			fmt.Fprintln(log, "ERRO:", fr.Err)
		case len(fr.Found) > 0:
			fmt.Fprintln(log, "indicador de IA removido")
		default:
			fmt.Fprintln(log, "sem indicador de IA")
		}
		sum.Results = append(sum.Results, fr)
	}
	for d := range outDirs {
		sum.OutDirs = append(sum.OutDirs, d)
	}
	sort.Strings(sum.OutDirs)
	if !dryRun && len(sum.OutDirs) > 0 {
		sum.Report = writeReport(sum)
	}
	return sum
}

// verify faz a validação original → processado.
func verify(c codec, orig, clean []byte, fr *FileResult) error {
	w1, h1, err := c.dims(orig)
	if err != nil {
		return err
	}
	w2, h2, err := c.dims(clean)
	if err != nil {
		return err
	}
	if w1 != w2 || h1 != h2 {
		return fmt.Errorf("dimensões diferem: %dx%d → %dx%d", w1, h1, w2, h2)
	}
	fr.Dims = fmt.Sprintf("%dx%d", w1, h1)
	if err := c.preserved(orig, clean); err != nil {
		return err
	}
	if c.name == "JPEG" && bytes.Contains(clean, []byte("jumb")) && bytes.Contains(bytes.ToLower(clean), []byte("c2pa")) {
		// só falha se estiver em segmento APP11/JUMBF de metadados (antes do SOS)
		if segs, _, err := jpegclean.Parse(clean); err == nil {
			for _, sg := range segs {
				if sg.Marker == 0xEB && bytes.Contains(bytes.ToLower(sg.Payload), []byte("c2pa")) {
					return fmt.Errorf("C2PA ainda detectável no arquivo processado")
				}
			}
		}
	}
	return nil
}

// ReportText monta o relatório legível (sem BOM).
func ReportText(s Summary, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "RELATÓRIO — Remoção de metadados de IA (%s)\n\n", now.Format("02/01/2006 15:04:05"))
	fmt.Fprintf(&b, "Fotos encontradas ........ %d\n", s.Total())
	fmt.Fprintf(&b, "Processados com sucesso .. %d\n", s.Total()-s.Errors())
	fmt.Fprintf(&b, "Continham indicador de IA  %d\n", s.WithAI())
	fmt.Fprintf(&b, "Com erro ................. %d\n", s.Errors())
	fmt.Fprintf(&b, "Indicador ainda presente . %d\n\n", s.Persist())
	for _, r := range s.Results {
		fmt.Fprintf(&b, "- %s [%s] %s\n", r.Name, r.Status, r.Dims)
		for _, x := range r.Found {
			fmt.Fprintf(&b, "    encontrado: %s\n", x)
		}
		for _, x := range r.Removed {
			fmt.Fprintf(&b, "    removido:   %s\n", x)
		}
		for _, x := range r.Remaining {
			fmt.Fprintf(&b, "    PERSISTE:   %s\n", x)
		}
		if r.Err != "" {
			fmt.Fprintf(&b, "    erro: %s\n", r.Err)
		}
	}
	b.WriteString("\nSó as fotos com indicação de IA são gravadas nesta pasta; as demais não precisavam de limpeza.\n")
	b.WriteString("Preservado byte a byte: dados da imagem (sem recompressão), Exif, ICC, IPTC/Photoshop, XMP (autoria/copyright) e demais segmentos.\n")
	b.WriteString("\n" + Notice + "\n\n" + Credit + "\n")
	return b.String()
}

// ReportCSV monta o CSV (BOM UTF-8, separador ";") para o Excel.
func ReportCSV(s Summary) []byte {
	var b bytes.Buffer
	b.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&b)
	w.Comma = ';'
	w.Write([]string{"arquivo", "status", "dimensoes", "encontrado", "removido", "persiste", "erro"})
	for _, r := range s.Results {
		w.Write([]string{csvSafe(r.Name), r.Status, r.Dims, strings.Join(r.Found, " | "), strings.Join(r.Removed, " | "), strings.Join(r.Remaining, " | "), csvSafe(r.Err)})
	}
	w.Flush()
	return b.Bytes()
}

func writeReport(s Summary) string {
	now := time.Now()
	ts := now.Format("20060102_150405")
	dir := s.OutDirs[0]
	// O_EXCL: duas execuções no mesmo segundo (um processo por arquivo selecionado
	// no Explorer) não sobrescrevem o relatório uma da outra.
	var txt, base string
	for i := 0; i < 1000; i++ {
		base = "RELATORIO_" + ts
		if i > 0 {
			base += fmt.Sprintf("_%d", i)
		}
		txt = filepath.Join(dir, base+".txt")
		if tf, err := os.OpenFile(txt, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); err == nil {
			_, _ = tf.WriteString("\xEF\xBB\xBF" + ReportText(s, now))
			tf.Close()
			break
		}
	}

	if f, err := os.Create(filepath.Join(dir, base+".csv")); err == nil {
		_, _ = f.Write(ReportCSV(s))
		f.Close()
	}
	return txt
}

// Resumo curto para exibir ao usuário.
func (s Summary) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fotos encontradas: %d\nCom indicador de IA (removido): %d\nCom erro: %d\nIndicador ainda presente: %d\n", s.Total(), s.WithAI(), s.Errors(), s.Persist())
	if s.WithAI() == 0 && s.Errors() == 0 {
		b.WriteString("\nNenhuma foto tinha indicação de IA; nada foi gravado.\n")
	}
	for _, d := range s.OutDirs {
		fmt.Fprintf(&b, "\nSaída: %s", d)
	}
	return b.String()
}
