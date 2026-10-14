// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// Package webpclean remove, no nível de chunks RIFF, os indicadores de IA
// generativa de arquivos WebP sem decodificar nem recomprimir a imagem: o
// manifest C2PA (chunk "C2PA") é retirado e o XMP (chunk "XMP ") é limpo;
// VP8/VP8L/ALPH/ANIM e demais chunks são copiados byte a byte.
package webpclean

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"limpaia/internal/jpegclean"
	"limpaia/internal/xmpai"
)

// Info é o mesmo relatório usado para JPEG.
type Info = jpegclean.Info

// IsWebP diz se os dados são um contêiner RIFF/WEBP.
func IsWebP(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

// Chunk é um chunk RIFF do WebP.
type Chunk struct {
	FourCC string
	Data   []byte
	Raw    []byte // FourCC + tamanho + dados + byte de preenchimento (se houver)
}

// Parse devolve os chunks dentro do RIFF e o que vier depois dele.
func Parse(data []byte) (chunks []Chunk, trailer []byte, err error) {
	if !IsWebP(data) {
		return nil, nil, errors.New("não é um WebP (cabeçalho RIFF/WEBP ausente)")
	}
	size := uint64(binary.LittleEndian.Uint32(data[4:]))
	if size < 4 || size+8 > uint64(len(data)) {
		return nil, nil, errors.New("WebP truncado (tamanho RIFF inválido)")
	}
	end := int(size) + 8
	for i := 12; i < end; {
		if i+8 > end {
			return nil, nil, fmt.Errorf("chunk truncado no offset %d", i)
		}
		l := uint64(binary.LittleEndian.Uint32(data[i+4:]))
		n := 8 + l + l&1
		if uint64(i)+8+l > uint64(end) {
			return nil, nil, fmt.Errorf("chunk com tamanho inválido no offset %d", i)
		}
		if uint64(i)+n > uint64(end) { // último chunk ímpar sem o byte de preenchimento
			n = 8 + l
		}
		chunks = append(chunks, Chunk{FourCC: string(data[i : i+4]), Data: data[i+8 : i+8+int(l)], Raw: data[i : i+int(n)]})
		i += int(n)
	}
	if len(chunks) == 0 {
		return nil, nil, errors.New("WebP sem chunks")
	}
	return chunks, data[end:], nil
}

func chunk(fourcc string, data []byte) []byte {
	b := make([]byte, 8, 9+len(data))
	copy(b, fourcc)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(data)))
	b = append(b, data...)
	if len(data)&1 == 1 {
		b = append(b, 0)
	}
	return b
}

// Clean remove os indicadores de IA e devolve o novo arquivo.
func Clean(data []byte) ([]byte, Info, error) {
	var info Info
	chunks, trailer, err := Parse(data)
	if err != nil {
		return nil, info, err
	}
	var body bytes.Buffer
	body.Grow(len(data))
	nC2PA := 0
	for _, c := range chunks {
		switch c.FourCC {
		case "C2PA":
			nC2PA++
			info.Changed = true
			continue
		case "XMP ":
			if xmpai.Marker.Match(c.Data) {
				info.Found = append(info.Found, "XMP: indicador de IA")
			}
			if n := xmpai.Strip(string(c.Data)); n != string(c.Data) {
				body.Write(chunk("XMP ", []byte(n)))
				info.Changed = true
				info.Removed = append(info.Removed, "XMP: DigitalSourceType de IA / crs:fill_method=firefly")
				continue
			}
		}
		body.Write(c.Raw)
	}
	if nC2PA > 0 {
		n := fmt.Sprintf("WebP C2PA (Content Credentials): %d chunk(s)", nC2PA)
		info.Found = append([]string{n}, info.Found...)
		info.Removed = append([]string{n}, info.Removed...)
	}
	if !info.Changed {
		return append([]byte{}, data...), info, nil
	}
	out := make([]byte, 12, 12+body.Len()+len(trailer))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(4+body.Len()))
	copy(out[8:], "WEBP")
	out = append(out, body.Bytes()...)
	out = append(out, trailer...)

	// reexame do resultado
	rc, _, err := Parse(out)
	if err != nil {
		return nil, info, fmt.Errorf("resultado inválido: %w", err)
	}
	for _, c := range rc {
		if c.FourCC == "C2PA" {
			info.Remaining = append(info.Remaining, "WebP C2PA")
		}
		if c.FourCC == "XMP " && xmpai.Marker.Match(c.Data) {
			info.Remaining = append(info.Remaining, "XMP contém marcador de IA")
		}
	}
	return out, info, nil
}

// Dimensions lê largura/altura (VP8X, VP8L ou VP8).
func Dimensions(data []byte) (w, h int, err error) {
	chunks, _, err := Parse(data)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range chunks {
		d := c.Data
		switch {
		case c.FourCC == "VP8X" && len(d) >= 10:
			u24 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
			return u24(d[4:]) + 1, u24(d[7:]) + 1, nil
		case c.FourCC == "VP8L" && len(d) >= 5 && d[0] == 0x2F:
			b := binary.LittleEndian.Uint32(d[1:])
			return int(b&0x3FFF) + 1, int(b>>14&0x3FFF) + 1, nil
		case c.FourCC == "VP8 " && len(d) >= 10 && d[3] == 0x9D && d[4] == 0x01 && d[5] == 0x2A:
			return int(binary.LittleEndian.Uint16(d[6:]) & 0x3FFF), int(binary.LittleEndian.Uint16(d[8:]) & 0x3FFF), nil
		}
	}
	return 0, 0, errors.New("dimensões do WebP não encontradas")
}

// PreservedEqual confere que tudo exceto os chunks C2PA e XMP permanece idêntico.
func PreservedEqual(orig, clean []byte) error {
	keep := func(b []byte) ([][]byte, []byte, error) {
		cs, tr, err := Parse(b)
		if err != nil {
			return nil, nil, err
		}
		var r [][]byte
		for _, c := range cs {
			if c.FourCC != "C2PA" && c.FourCC != "XMP " {
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
