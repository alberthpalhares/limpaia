// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// mkmac monta o pacote para macOS a partir dos dois executáveis Go:
// funde arm64 + x86_64 num binário universal (equivalente ao `lipo -create`),
// coloca-o dentro de LimpaIA.app e grava um .zip com permissões Unix
// (o Windows não grava o bit de execução, e sem ele o app não abre no Mac).
//
// Uso: mkmac -arm64 <arquivo> -amd64 <arquivo> [-icns <icns>] [-versao X.Y.Z] -leiame <txt> -out <zip>
package main

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
)

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key><string>pt-BR</string>
	<key>CFBundleExecutable</key><string>LimpaIA</string>
	<key>CFBundleIdentifier</key><string>app.limpaia.LimpaIA</string>
	<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
	<key>CFBundleName</key><string>LimpaIA</string>
	<key>CFBundleDisplayName</key><string>LimpaIA</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>%[2]s</string>
	<key>CFBundleVersion</key><string>%[2]s</string>
	<key>CFBundleIconFile</key><string>LimpaIA</string>
	<key>NSHumanReadableCopyright</key><string>Copyright 2026 Alberth Palhares · Apache-2.0</string>
	<key>LSMinimumSystemVersion</key><string>%[1]s</string>
	<key>LSUIElement</key><true/>
	<key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`

type slice struct {
	data     []byte
	cpu, sub uint32
	align    uint32 // log2
	minos    string
}

func load(path string) slice {
	b, err := os.ReadFile(path)
	check(err)
	f, err := macho.NewFile(bytes.NewReader(b))
	check(err)
	s := slice{data: b, cpu: uint32(f.Cpu), sub: f.SubCpu}
	switch f.Cpu {
	case macho.CpuArm64:
		s.align = 14
	case macho.CpuAmd64:
		s.align = 12
	default:
		check(fmt.Errorf("%s: arquitetura inesperada %v", path, f.Cpu))
	}
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) >= 16 && f.ByteOrder.Uint32(raw) == 0x32 { // LC_BUILD_VERSION
			v := f.ByteOrder.Uint32(raw[12:])
			s.minos = fmt.Sprintf("%d.%d", v>>16, v>>8&0xff)
		}
	}
	return s
}

// fat devolve o arquivo Mach-O universal (cabeçalho big-endian, fatias alinhadas).
func fat(slices ...slice) []byte {
	var hdr bytes.Buffer
	w := func(v uint32) { binary.Write(&hdr, binary.BigEndian, v) }
	w(0xCAFEBABE)
	w(uint32(len(slices)))
	off := uint32(0)
	offs := make([]uint32, len(slices))
	end := uint32(8 + 20*len(slices))
	for i, s := range slices {
		a := uint32(1) << s.align
		off = (end + a - 1) &^ (a - 1)
		offs[i] = off
		end = off + uint32(len(s.data))
		w(s.cpu)
		w(s.sub)
		w(off)
		w(uint32(len(s.data)))
		w(s.align)
	}
	out := make([]byte, end)
	copy(out, hdr.Bytes())
	for i, s := range slices {
		copy(out[offs[i]:], s.data)
	}
	return out
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkmac:", err)
		os.Exit(1)
	}
}

func main() {
	arm := flag.String("arm64", "", "executável darwin/arm64")
	amd := flag.String("amd64", "", "executável darwin/amd64")
	readme := flag.String("leiame", "", "texto de instruções")
	icns := flag.String("icns", "", "ícone do app (.icns)")
	ver := flag.String("versao", "0.0.0", "versão do app (X.Y.Z)")
	out := flag.String("out", "", "zip de saída")
	flag.Parse()
	if *arm == "" || *amd == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	a, x := load(*arm), load(*amd)
	minos := a.minos
	if x.minos > minos {
		minos = x.minos
	}
	if minos == "" {
		minos = "11.0"
	}
	universal := fat(x, a) // mesma ordem do lipo: x86_64, arm64

	// confere que o resultado é lido como universal com as duas arquiteturas
	ff, err := macho.NewFatFile(bytes.NewReader(universal))
	check(err)
	if len(ff.Arches) != 2 {
		check(fmt.Errorf("fat com %d arquiteturas", len(ff.Arches)))
	}

	f, err := os.Create(*out)
	check(err)
	zw := zip.NewWriter(f)
	add := func(name string, mode os.FileMode, data []byte) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		check(err)
		if data != nil {
			_, err = w.Write(data)
			check(err)
		}
	}
	add("LimpaIA.app/", os.ModeDir|0o755, nil)
	add("LimpaIA.app/Contents/", os.ModeDir|0o755, nil)
	add("LimpaIA.app/Contents/MacOS/", os.ModeDir|0o755, nil)
	add("LimpaIA.app/Contents/Info.plist", 0o644, []byte(fmt.Sprintf(infoPlist, minos, *ver)))
	if *icns != "" {
		b, err := os.ReadFile(*icns)
		check(err)
		add("LimpaIA.app/Contents/Resources/", os.ModeDir|0o755, nil)
		add("LimpaIA.app/Contents/Resources/LimpaIA.icns", 0o644, b)
	}
	add("LimpaIA.app/Contents/MacOS/LimpaIA", 0o755, universal)
	if *readme != "" {
		b, err := os.ReadFile(*readme)
		check(err)
		add("LEIA-ME.txt", 0o644, b)
	}
	check(zw.Close())
	check(f.Close())
	fmt.Printf("%s: LimpaIA.app universal (arm64+x86_64), macOS %s ou mais novo, %d bytes\n", *out, minos, len(universal))
}
