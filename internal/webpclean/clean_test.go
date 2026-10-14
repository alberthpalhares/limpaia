// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

package webpclean

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"testing"
)

// WebP sem perdas 1x1 válido (o menor possível).
var tiny, _ = base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")

const aiXMP = `<rdf:Description xmlns:Iptc4xmpExt="http://iptc.org/std/Iptc4xmpExt/2008-02-29/" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
	`Iptc4xmpExt:DigitalSourceType="http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia" dc:rights="© Fulano"/>`

// extended monta um WebP estendido: VP8X + VP8L do tiny + XMP + C2PA (tamanho ímpar).
func extended(xmp string, withC2PA bool) []byte {
	cs, _, err := Parse(tiny)
	if err != nil {
		panic(err)
	}
	vp8x := make([]byte, 10)
	vp8x[0] = 0x04 // flag de XMP
	var body []byte
	body = append(body, chunk("VP8X", vp8x)...)
	body = append(body, cs[0].Raw...)
	body = append(body, chunk("XMP ", []byte(xmp))...)
	if withC2PA {
		body = append(body, chunk("C2PA", []byte("\x00\x00\x00\x21jumb\x00\x00\x00\x19jumdc2pa\x00"))...)
	}
	out := append([]byte("RIFF\x00\x00\x00\x00WEBP"), body...)
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out
}

func TestCleanC2PAAndXMP(t *testing.T) {
	in := extended(aiXMP, true)
	out, info, err := Clean(in)
	if err != nil || !info.Changed || len(info.Found) != 2 || len(info.Remaining) != 0 {
		t.Fatalf("err=%v info=%+v", err, info)
	}
	if bytes.Contains(out, []byte("C2PA")) || bytes.Contains(out, []byte("trainedAlgorithmic")) || !bytes.Contains(out, []byte("© Fulano")) {
		t.Fatal("C2PA/XMP de IA continuam, ou o copyright sumiu")
	}
	if int(binary.LittleEndian.Uint32(out[4:]))+8 != len(out) {
		t.Fatal("tamanho RIFF errado")
	}
	if err := PreservedEqual(in, out); err != nil {
		t.Fatal(err)
	}
	cs, _, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if len(c.Raw)%2 != 0 {
			t.Fatalf("chunk %q sem preenchimento par", c.FourCC)
		}
	}
	if w, h, err := Dimensions(out); err != nil || w != 1 || h != 1 {
		t.Fatalf("dimensões %dx%d %v", w, h, err)
	}
}

func TestNoAIUnchanged(t *testing.T) {
	for _, in := range [][]byte{tiny, extended(`<rdf:Description dc:rights="© Fulano"/>`, false)} {
		out, info, err := Clean(in)
		if err != nil || info.Changed || !bytes.Equal(in, out) {
			t.Fatalf("err=%v info=%+v", err, info)
		}
	}
}

func TestDimensionsSimple(t *testing.T) {
	if w, h, err := Dimensions(tiny); err != nil || w != 1 || h != 1 {
		t.Fatalf("%dx%d %v", w, h, err)
	}
}

func TestTruncatedNoPanic(t *testing.T) {
	in := extended(aiXMP, true)
	for i := 0; i < len(in); i++ {
		Clean(in[:i])
	}
}

func FuzzClean(f *testing.F) {
	f.Add(extended(aiXMP, true))
	f.Fuzz(func(t *testing.T, b []byte) {
		out, _, err := Clean(b)
		if err == nil {
			if _, _, err := Parse(out); err != nil {
				t.Fatalf("saída inválida: %v", err)
			}
		}
	})
}
