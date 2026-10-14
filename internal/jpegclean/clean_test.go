package jpegclean

import (
	"bytes"
	"testing"
)

func seg(m byte, payload []byte) []byte {
	l := len(payload) + 2
	return append([]byte{0xFF, m, byte(l >> 8), byte(l)}, payload...)
}

func jumb(en, z byte) []byte {
	p := []byte{'J', 'P', 0, en, 0, 0, 0, z}
	p = append(p, 0, 0, 0, 16, 'j', 'u', 'm', 'b', 'c', '2', 'p', 'a')
	return seg(mAPP11, p)
}

func build(parts ...[]byte) []byte {
	b := []byte{0xFF, mSOI}
	for _, p := range parts {
		b = append(b, p...)
	}
	sof := seg(0xC0, []byte{8, 0, 10, 0, 20, 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})
	b = append(b, sof...)
	b = append(b, seg(mSOS, []byte{3, 1, 0, 2, 0x11, 3, 0x11, 0, 63, 0})...)
	b = append(b, 0x12, 0x34, 0xFF, 0x00, 0x56, 0xFF, 0xD9)
	return b
}

func TestRemovesC2PAFragmentsOnly(t *testing.T) {
	exif := seg(mAPP1, append([]byte("Exif\x00\x00"), 'x', 'y'))
	icc := seg(mAPP2, []byte("ICC_PROFILE\x00\x01\x01abc"))
	iptc := seg(mAPP13, []byte("Photoshop 3.0\x00data"))
	other := jumb(7, 9) // outra instância JUMBF
	in := build(exif, jumb(1, 1), jumb(1, 2), icc, iptc, other)
	out, info, err := Clean(in)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Changed || len(info.Remaining) != 0 {
		t.Fatalf("info=%+v", info)
	}
	if bytes.Contains(out, []byte("jumb")) {
		t.Fatal("C2PA restante")
	}
	if !bytes.Contains(out, exif) || !bytes.Contains(out, icc) || !bytes.Contains(out, iptc) {
		t.Fatal("segmento preservado ausente")
	}
	if err := PreservedEqual(in, out); err != nil {
		t.Fatal(err)
	}
	if w, h, _ := Dimensions(out); w != 20 || h != 10 {
		t.Fatalf("dim %dx%d", w, h)
	}
}

func TestXMPDigitalSourceType(t *testing.T) {
	x := `<x:xmpmeta><rdf:Description dc:format="image/jpeg" Iptc4xmpExt:DigitalSourceType="http://cv.iptc.org/newscodes/digitalsourcetype/compositeWithTrainedAlgorithmicMedia"><dc:rights>Copy</dc:rights></rdf:Description></x:xmpmeta>`
	in := build(seg(mAPP1, append(append([]byte{}, xmpStd...), x...)))
	out, info, err := Clean(in)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Changed || bytes.Contains(bytes.ToLower(out), []byte("trainedalgorithmicmedia")) {
		t.Fatalf("não removeu: %+v", info)
	}
	if !bytes.Contains(out, []byte("<dc:rights>Copy</dc:rights>")) || !bytes.Contains(out, []byte(`dc:format="image/jpeg"`)) {
		t.Fatal("perdeu metadado legítimo")
	}
}

func TestNoAIUnchanged(t *testing.T) {
	in := build(seg(mAPP1, []byte("Exif\x00\x00zz")))
	out, info, err := Clean(in)
	if err != nil || info.Changed || !bytes.Equal(in, out) {
		t.Fatalf("deveria ser idêntico: %v %+v", err, info)
	}
}

func TestLightroomFireflyFill(t *testing.T) {
	x := `<rdf:Description crs:fill_method="firefly" crs:pm_black_level="2048" dc:format="image/jpeg"/>`
	in := build(seg(mAPP1, append(append([]byte{}, xmpStd...), x...)))
	out, info, err := Clean(in)
	if err != nil || !info.Changed || len(info.Remaining) != 0 {
		t.Fatalf("%v %+v", err, info)
	}
	if bytes.Contains(bytes.ToLower(out), []byte("firefly")) || !bytes.Contains(out, []byte(`crs:pm_black_level="2048"`)) {
		t.Fatal("remoção incorreta")
	}
}
