// mkweb monta a versão web num único HTML autocontido: insere no modelo o
// wasm_exec.js do Go (o mesmo toolchain que compilou o WASM) e o WASM
// comprimido com gzip em base64 (a página descompacta com DecompressionStream).
// Abre direto do disco (file://), sem servidor e sem internet.
//
// Uso: mkweb -wasm <arquivo.wasm> -modelo <index.html> -out <LimpaIA.html> [-wasmexec <wasm_exec.js>]
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkweb:", err)
		os.Exit(1)
	}
}

// replaceOnce exige exatamente um marcador, para não gerar página quebrada em silêncio.
func replaceOnce(s, marker, value string) string {
	if n := strings.Count(s, marker); n != 1 {
		check(fmt.Errorf("marcador %s aparece %d vezes no modelo", marker, n))
	}
	return strings.Replace(s, marker, value, 1)
}

func main() {
	wasm := flag.String("wasm", "", "WASM compilado com GOOS=js GOARCH=wasm")
	tmpl := flag.String("modelo", "", "modelo HTML (web/index.html)")
	execJS := flag.String("wasmexec", "", "wasm_exec.js (padrão: o do GOROOT do go no PATH)")
	out := flag.String("out", "", "HTML de saída")
	flag.Parse()
	if *wasm == "" || *tmpl == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *execJS == "" {
		root, err := exec.Command("go", "env", "GOROOT").Output()
		check(err)
		*execJS = filepath.Join(strings.TrimSpace(string(root)), "lib", "wasm", "wasm_exec.js")
	}

	bin, err := os.ReadFile(*wasm)
	check(err)
	if !bytes.HasPrefix(bin, []byte("\x00asm")) {
		check(fmt.Errorf("%s não é um módulo WebAssembly", *wasm))
	}
	var gz bytes.Buffer
	zw, err := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	check(err)
	_, err = zw.Write(bin)
	check(err)
	check(zw.Close())

	js, err := os.ReadFile(*execJS)
	check(err)
	if bytes.Contains(bytes.ToLower(js), []byte("</script")) {
		check(fmt.Errorf("%s contém </script e quebraria a página", *execJS))
	}
	page, err := os.ReadFile(*tmpl)
	check(err)

	html := replaceOnce(string(page), "/*WASM_EXEC_JS*/", string(js))
	html = replaceOnce(html, "/*WASM_GZ_BASE64*/", base64.StdEncoding.EncodeToString(gz.Bytes()))
	check(os.WriteFile(*out, []byte(html), 0o644))
	fmt.Printf("%s: página autocontida, %d bytes (WASM %d -> gzip %d)\n", *out, len(html), len(bin), gz.Len())
}
