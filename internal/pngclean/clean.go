// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// Package pngclean remove, no nível de chunks PNG, os indicadores de IA
// generativa sem decodificar nem recomprimir a imagem: o manifest C2PA (chunk
// caBX) é retirado e o XMP (iTXt "XML:com.adobe.xmp") é limpo; IDAT e demais
// chunks são copiados byte a byte.
package pngclean

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"

	"limpaia/internal/jpegclean"
	"limpaia/internal/xmpai"
)

// Info é o mesmo relatório usado para JPEG.
type Info = jpegclean.Info

var sig = []byte("\x89PNG\r\n\x1a\n")

const xmpKeyword = "XML:com.adobe.xmp"

// limite do XMP descomprimido (o tamanho vem do arquivo)
const maxXMP = 64 << 20

// IsPNG diz se os dados começam com a assinatura PNG.
func IsPNG(data []byte) bool { return bytes.HasPrefix(data, sig) }

// Chunk é um chunk PNG.
type Chunk struct {
	Type string
	Data []byte
	Raw  []byte // comprimento + tipo + dados + CRC
}

// Parse devolve os chunks até o IEND (inclusive) e o que vier depois dele.
func Parse(data []byte) (chunks []Chunk, trailer []byte, err error) {
	if !IsPNG(data) {
		return nil, nil, errors.New("não é um PNG (assinatura ausente)")
	}
	i := len(sig)
	for {
		if i+12 > len(data) {
			return nil, nil, errors.New("PNG truncado antes do IEND")
		}
		l := binary.BigEndian.Uint32(data[i:])
		if l > 0x7FFFFFFF || uint64(i)+12+uint64(l) > uint64(len(data)) {
			return nil, nil, fmt.Errorf("chunk com tamanho inválido no offset %d", i)
		}
		end := i + 12 + int(l)
		c := Chunk{Type: string(data[i+4 : i+8]), Data: data[i+8 : i+8+int(l)], Raw: data[i:end]}
		chunks = append(chunks, c)
		i = end
		if c.Type == "IEND" {
			return chunks, data[i:], nil
		}
	}
}

func chunk(typ string, data []byte) []byte {
	b := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:], typ)
	b = append(b, data...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b[4:]))
}

// itxt separa um iTXt em cabeçalho (até o texto) e texto (já descomprimido).
func itxt(d []byte) (keyword string, head []byte, compressed bool, text []byte, err error) {
	k := bytes.IndexByte(d, 0)
	if k < 0 || k+3 > len(d) {
		return "", nil, false, nil, errors.New("iTXt malformado")
	}
	keyword = string(d[:k])
	compressed = d[k+1] == 1
	rest := d[k+3:]
	lang := bytes.IndexByte(rest, 0)
	if lang < 0 {
		return "", nil, false, nil, errors.New("iTXt malformado")
	}
	tr := bytes.IndexByte(rest[lang+1:], 0)
	if tr < 0 {
		return "", nil, false, nil, errors.New("iTXt malformado")
	}
	hl := k + 3 + lang + 1 + tr + 1
	head, text = d[:hl], d[hl:]
	if compressed {
		r, err := zlib.NewReader(bytes.NewReader(text))
		if err != nil {
			return "", nil, false, nil, fmt.Errorf("iTXt comprimido inválido: %w", err)
		}
		text, err = io.ReadAll(io.LimitReader(r, maxXMP+1))
		if err != nil || len(text) > maxXMP {
			return "", nil, false, nil, errors.New("iTXt comprimido inválido ou grande demais")
		}
	}
	return keyword, head, compressed, text, nil
}

func isXMP(c Chunk) bool {
	return c.Type == "iTXt" && bytes.HasPrefix(c.Data, []byte(xmpKeyword+"\x00"))
}

// Clean remove os indicadores de IA e devolve o novo arquivo.
func Clean(data []byte) ([]byte, Info, error) {
	var info Info
	chunks, trailer, err := Parse(data)
	if err != nil {
		return nil, info, err
	}
	var out bytes.Buffer
	out.Grow(len(data))
	out.Write(sig)
	nC2PA := 0
	for _, c := range chunks {
		switch {
		case c.Type == "caBX":
			nC2PA++
			info.Changed = true
			continue
		case isXMP(c):
			_, head, compressed, text, err := itxt(c.Data)
			if err != nil { // XMP ilegível: fica como está (o reexame aponta se sobrar marca)
				out.Write(c.Raw)
				continue
			}
			if xmpai.Marker.Match(text) {
				info.Found = append(info.Found, "XMP: indicador de IA")
			}
			n := xmpai.Strip(string(text))
			if n != string(text) {
				body := []byte(n)
				if compressed {
					var z bytes.Buffer
					w, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
					w.Write(body)
					w.Close()
					body = z.Bytes()
				}
				out.Write(chunk("iTXt", append(append([]byte{}, head...), body...)))
				info.Changed = true
				info.Removed = append(info.Removed, "XMP: DigitalSourceType de IA / crs:fill_method=firefly")
				continue
			}
		}
		out.Write(c.Raw)
	}
	if nC2PA > 0 {
		n := fmt.Sprintf("PNG caBX (C2PA / Content Credentials): %d chunk(s)", nC2PA)
		info.Found = append([]string{n}, info.Found...)
		info.Removed = append([]string{n}, info.Removed...)
	}
	out.Write(trailer)
	res := out.Bytes()

	// reexame do resultado
	rc, _, err := Parse(res)
	if err != nil {
		return nil, info, fmt.Errorf("resultado inválido: %w", err)
	}
	for _, c := range rc {
		if c.Type == "caBX" {
			info.Remaining = append(info.Remaining, "PNG caBX")
		}
		if isXMP(c) {
			if _, _, _, text, err := itxt(c.Data); err == nil && xmpai.Marker.Match(text) {
				info.Remaining = append(info.Remaining, "XMP contém marcador de IA")
			}
		}
	}
	return res, info, nil
}

// Dimensions lê largura/altura do IHDR.
func Dimensions(data []byte) (w, h int, err error) {
	chunks, _, err := Parse(data)
	if err != nil {
		return 0, 0, err
	}
	if len(chunks) == 0 || chunks[0].Type != "IHDR" || len(chunks[0].Data) < 8 {
		return 0, 0, errors.New("IHDR não encontrado")
	}
	d := chunks[0].Data
	return int(binary.BigEndian.Uint32(d)), int(binary.BigEndian.Uint32(d[4:])), nil
}

// PreservedEqual confere que tudo exceto os itens removidos/limpos permanece
// idêntico: IHDR, IDAT, paleta, ICC, Exif, textos e demais chunks, na ordem.
func PreservedEqual(orig, clean []byte) error {
	keep := func(b []byte) ([][]byte, []byte, error) {
		cs, tr, err := Parse(b)
		if err != nil {
			return nil, nil, err
		}
		var r [][]byte
		for _, c := range cs {
			if c.Type != "caBX" && !isXMP(c) {
				r = append(r, c.Raw)
			}
		}
		return r, tr, nil
	}
	a, ta, err := keep(orig)
	if err != nil {
		return err
	}
	b, tb, err := keep(clean)
	if err != nil {
		return err
	}
	if len(a) != len(b) || !bytes.Equal(ta, tb) {
		return errors.New("número de chunks preservados difere")
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return fmt.Errorf("chunk preservado #%d difere", i)
		}
	}
	return nil
}
