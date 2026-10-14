// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

package pngclean

import (
	"bytes"
	"compress/zlib"
	"image"
	"image/color"
	"image/png"
	"testing"
)

const aiXMP = `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
	`<rdf:Description xmlns:Iptc4xmpExt="http://iptc.org/std/Iptc4xmpExt/2008-02-29/" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
	`Iptc4xmpExt:DigitalSourceType="http://cv.iptc.org/newscodes/digitalsourcetype/compositeWithTrainedAlgorithmicMedia" dc:rights="© Fulano"/>` +
	`</rdf:RDF></x:xmpmeta>`

// base devolve um PNG real 3x2 codificado pelo image/png.
func base(t *testing.T) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 37)
	}
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func xmpChunk(compressed bool, text string) []byte {
	body := []byte(text)
	flag := byte(0)
	if compressed {
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		w.Write(body)
		w.Close()
		body, flag = z.Bytes(), 1
	}
	d := append([]byte(xmpKeyword+"\x00"), flag, 0)
	d = append(d, "\x00\x00"...) // idioma e palavra-chave traduzida vazios
	return chunk("iTXt", append(d, body...))
}

// inject coloca chunks logo depois do IHDR.
func inject(p []byte, extra ...[]byte) []byte {
	ihdrEnd := len(sig) + 12 + 13
	out := append([]byte{}, p[:ihdrEnd]...)
	for _, e := range extra {
		out = append(out, e...)
	}
	return append(out, p[ihdrEnd:]...)
}

func idat(t *testing.T, b []byte) []byte {
	cs, _, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	var r []byte
	for _, c := range cs {
		if c.Type == "IDAT" {
			r = append(r, c.Raw...)
		}
	}
	return r
}

func TestCleanC2PAAndXMP(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		orig := base(t)
		in := inject(orig, chunk("caBX", []byte("\x00\x00\x00\x20jumb\x00\x00\x00\x18jumdc2pa")), xmpChunk(compressed, aiXMP))
		out, info, err := Clean(in)
		if err != nil || !info.Changed || len(info.Found) != 2 || len(info.Remaining) != 0 {
			t.Fatalf("compressed=%v: err=%v info=%+v", compressed, err, info)
		}
		if bytes.Contains(out, []byte("caBX")) {
			t.Fatal("caBX continua no arquivo")
		}
		if !bytes.Equal(idat(t, in), idat(t, out)) {
			t.Fatal("IDAT foi alterado")
		}
		if err := PreservedEqual(in, out); err != nil {
			t.Fatal(err)
		}
		// image/png confere os CRCs: o arquivo tem que continuar válido
		if _, err := png.Decode(bytes.NewReader(out)); err != nil {
			t.Fatalf("PNG inválido: %v", err)
		}
		cs, _, _ := Parse(out)
		for _, c := range cs {
			if isXMP(c) {
				_, _, comp, text, err := itxt(c.Data)
				if err != nil || comp != compressed || bytes.Contains(text, []byte("TrainedAlgorithmic")) || !bytes.Contains(text, []byte("© Fulano")) {
					t.Fatalf("XMP: comp=%v err=%v %s", comp, err, text)
				}
			}
		}
		if w, h, err := Dimensions(out); err != nil || w != 3 || h != 2 {
			t.Fatalf("dimensões %dx%d %v", w, h, err)
		}
	}
}

func TestNoAIUnchanged(t *testing.T) {
	in := inject(base(t), xmpChunk(false, `<rdf:Description dc:rights="© Fulano"/>`))
	out, info, err := Clean(in)
	if err != nil || info.Changed || len(info.Found) != 0 || !bytes.Equal(in, out) {
		t.Fatalf("err=%v info=%+v igual=%v", err, info, bytes.Equal(in, out))
	}
}

func TestTruncatedNoPanic(t *testing.T) {
	in := inject(base(t), chunk("caBX", []byte("jumbc2pa")), xmpChunk(true, aiXMP))
	for i := 0; i < len(in); i++ {
		Clean(in[:i])
	}
	if _, _, err := Clean([]byte("não é png")); err == nil {
		t.Fatal("esperava erro")
	}
}

func FuzzClean(f *testing.F) {
	f.Add(inject(base(&testing.T{}), chunk("caBX", []byte("jumbc2pa")), xmpChunk(true, aiXMP)))
	f.Fuzz(func(t *testing.T, b []byte) {
		out, _, err := Clean(b)
		if err == nil {
			if _, _, err := Parse(out); err != nil {
				t.Fatalf("saída inválida: %v", err)
			}
		}
	})
}
