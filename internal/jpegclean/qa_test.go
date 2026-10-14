package jpegclean

// Testes de QA (robustez/segurança). Testes "Regress_" reproduzem defeitos
// achados na revisão de QA e já corrigidos.

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

func noPanic(t *testing.T, data []byte) (out []byte, info Info, err error, panicked any) {
	t.Helper()
	defer func() { panicked = recover() }()
	out, info, err = Clean(data)
	return
}

func guidOf(b []byte) string {
	s := md5.Sum(b)
	return strings.ToUpper(hex.EncodeToString(s[:]))
}

func extChunk(guid string, total, off uint32, data []byte) []byte {
	p := append([]byte{}, xmpExt...)
	p = append(p, guid...)
	var w [8]byte
	binary.BigEndian.PutUint32(w[0:], total)
	binary.BigEndian.PutUint32(w[4:], off)
	p = append(p, w[:]...)
	p = append(p, data...)
	return seg(mAPP1, p)
}

func extSegments(content []byte, size int) [][]byte {
	g := guidOf(content)
	var r [][]byte
	for off := 0; off < len(content); off += size {
		end := min(off+size, len(content))
		r = append(r, extChunk(g, uint32(len(content)), uint32(off), content[off:end]))
	}
	return r
}

const rdfNS = `xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"`

func mainXMP(guid string) []byte {
	x := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF ` + rdfNS + `><rdf:Description rdf:about="" ` +
		`xmlns:xmpNote="http://ns.adobe.com/xmp/note/" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmpNote:HasExtendedXMP="` + guid + `"><dc:creator><rdf:Seq><rdf:li>Fotógrafo Fulano</rdf:li></rdf:Seq></dc:creator>` +
		`<dc:rights><rdf:Alt><rdf:li xml:lang="x-default">© Fulano</rdf:li></rdf:Alt></dc:rights></rdf:Description></rdf:RDF></x:xmpmeta>`
	return seg(mAPP1, append(append([]byte{}, xmpStd...), x...))
}

func extContent(items int) []byte {
	var b strings.Builder
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF ` + rdfNS + `><rdf:Description rdf:about="" ` +
		`xmlns:crs="http://ns.adobe.com/camera-raw-settings/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:description>Pousada &amp; restaurante</dc:description><crs:RetouchAreas><rdf:Seq>`)
	for i := 0; i < items; i++ {
		b.WriteString(`<rdf:li crs:SpotType="heal" crs:fill_method="firefly" crs:Opacity="1" crs:Feather="0.5" crs:Seed="12345678"/>` + "\n")
	}
	b.WriteString(`</rdf:Seq></crs:RetouchAreas></rdf:Description></rdf:RDF></x:xmpmeta>`)
	return []byte(b.String())
}

func wellFormed(t *testing.T, b []byte) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(b))
	for {
		if _, err := d.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("XML malformado: %v", err)
		}
	}
}

// reassemble remonta o XMP estendido do resultado e confere GUID/total/offset.
func reassemble(t *testing.T, out []byte) (guidMain string, content []byte) {
	t.Helper()
	segs, _, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	var total uint32
	var guids = map[string]bool{}
	cover := 0
	for _, s := range segs {
		if s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpStd) {
			i := bytes.Index(s.Payload, []byte(`HasExtendedXMP="`))
			if i < 0 {
				t.Fatal("HasExtendedXMP ausente")
			}
			guidMain = string(s.Payload[i+16 : i+48])
		}
		if s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpExt) {
			p := s.Payload[len(xmpExt):]
			guids[string(p[:32])] = true
			tt := binary.BigEndian.Uint32(p[32:36])
			if content == nil {
				total, content = tt, make([]byte, tt)
			} else if tt != total {
				t.Fatalf("total inconsistente %d != %d", tt, total)
			}
			off := binary.BigEndian.Uint32(p[36:40])
			d := p[40:]
			if int(off)+len(d) > int(total) {
				t.Fatal("chunk excede o total")
			}
			copy(content[off:], d)
			cover += len(d)
		}
	}
	if len(guids) != 1 || !guids[guidMain] {
		t.Fatalf("GUIDs dos chunks %v != GUID principal %s", guids, guidMain)
	}
	if cover != int(total) {
		t.Fatalf("cobertura %d != total %d", cover, total)
	}
	if g := guidOf(content); g != guidMain {
		t.Fatalf("GUID %s != MD5 do conteúdo %s", guidMain, g)
	}
	return
}

func TestQA_ExtendedXMPRoundTrip(t *testing.T) {
	content := extContent(3000) // ~330 KB -> vários chunks
	ext := extSegments(content, 65400)
	parts := [][]byte{seg(mAPP1, []byte("Exif\x00\x00II*\x00")), mainXMP(guidOf(content))}
	parts = append(parts, ext...)
	parts = append(parts, seg(mAPP2, []byte("ICC_PROFILE\x00\x01\x01icc")))
	in := build(parts...)
	out, info, err, p := noPanic(t, in)
	if p != nil || err != nil {
		t.Fatalf("panic=%v err=%v", p, err)
	}
	if !info.Changed || len(info.Remaining) != 0 {
		t.Fatalf("info=%+v", info)
	}
	_, c := reassemble(t, out)
	wellFormed(t, c)
	if bytes.Contains(bytes.ToLower(c), []byte("firefly")) {
		t.Fatal("firefly restante no XMP estendido")
	}
	if bytes.Count(c, []byte(`crs:SpotType="heal"`)) != 3000 || !bytes.Contains(c, []byte("Pousada &amp; restaurante")) {
		t.Fatal("conteúdo legítimo do XMP estendido perdido")
	}
	if !bytes.Contains(out, []byte("Fotógrafo Fulano")) || !bytes.Contains(out, []byte("© Fulano")) {
		t.Fatal("dc:creator/dc:rights perdidos")
	}
	if err := PreservedEqual(in, out); err != nil {
		t.Fatal(err)
	}
	// idempotência
	out2, info2, err := Clean(out)
	if err != nil || info2.Changed || !bytes.Equal(out, out2) {
		t.Fatalf("segunda passada alterou o arquivo: %v %+v", err, info2)
	}
}

func TestQA_SegmentLengthEdgeCases(t *testing.T) {
	sof := seg(0xC0, []byte{8, 0, 10, 0, 20, 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})
	sos := seg(mSOS, []byte{3, 1, 0, 2, 0x11, 3, 0x11, 0, 63, 0})
	tail := append(append(append([]byte{}, sof...), sos...), 0x11, 0xFF, 0xD9)
	cases := map[string][]byte{
		"len0":           append([]byte{0xFF, mSOI, 0xFF, 0xE1, 0, 0}, tail...),
		"len1":           append([]byte{0xFF, mSOI, 0xFF, 0xE1, 0, 1}, tail...),
		"len65535-trunc": append([]byte{0xFF, mSOI, 0xFF, 0xE1, 0xFF, 0xFF}, tail...),
		"so-SOI":         {0xFF, mSOI},
		"SOI+FF":         {0xFF, mSOI, 0xFF},
		"vazio":          {},
		"sem-SOS":        {0xFF, mSOI, 0xFF, 0xE0, 0, 4, 'a', 'b', 0xFF, 0xD9},
		"lixo":           []byte("not a jpeg at all"),
	}
	for n, c := range cases {
		_, _, err, p := noPanic(t, c)
		if p != nil {
			t.Errorf("%s: panic %v", n, p)
		}
		if err == nil {
			t.Errorf("%s: deveria dar erro", n)
		}
	}
	// len 2 (payload vazio) e 65535 completo são válidos
	full := make([]byte, 65533)
	for _, c := range [][]byte{build(seg(0xE5, nil)), build(seg(0xE5, full))} {
		out, info, err, p := noPanic(t, c)
		if p != nil || err != nil || info.Changed || !bytes.Equal(out, c) {
			t.Errorf("segmento válido: panic=%v err=%v changed=%v", p, err, info.Changed)
		}
	}
}

func TestQA_TruncatedPrefixesNoPanic(t *testing.T) {
	content := extContent(800)
	parts := [][]byte{seg(mAPP1, []byte("Exif\x00\x00II*\x00")), jumb(1, 1), jumb(1, 2), mainXMP(guidOf(content))}
	parts = append(parts, extSegments(content, 30000)...)
	in := build(parts...)
	for n := 0; n <= len(in); n++ {
		if _, _, _, p := noPanic(t, in[:n]); p != nil {
			t.Fatalf("panic com prefixo de %d bytes: %v", n, p)
		}
	}
}

func TestQA_NoEOIAndTrailingData(t *testing.T) {
	in := build(seg(mAPP1, []byte("Exif\x00\x00zz")))
	noEOI := in[:len(in)-2]
	out, info, err := Clean(noEOI)
	if err != nil || info.Changed || !bytes.Equal(out, noEOI) {
		t.Fatalf("sem EOI deveria sair idêntico: %v %+v", err, info)
	}
	trailing := append(append([]byte{}, in...), []byte("dados apos EOI")...)
	out, _, err = Clean(trailing)
	if err != nil || !bytes.Equal(out, trailing) {
		t.Fatal("dados após EOI deveriam ser preservados")
	}
}

func TestQA_MultipleManifestsAndNonJPApp11(t *testing.T) {
	nonJP := seg(mAPP11, []byte("HDR_RI ver=11\x00payload"))
	in := build(jumb(1, 1), jumb(1, 2), jumb(2, 1), seg(mAPP2, []byte("ICC_PROFILE\x00\x01\x01abc")), jumb(2, 2), nonJP)
	out, info, err := Clean(in)
	if err != nil || !info.Changed || len(info.Remaining) != 0 {
		t.Fatalf("%v %+v", err, info)
	}
	if bytes.Contains(out, []byte("jumb")) {
		t.Fatal("JUMBF restante")
	}
	if !bytes.Contains(out, nonJP) {
		t.Fatal("APP11 não-JUMBF foi removido")
	}
	if err := PreservedEqual(in, out); err != nil {
		t.Fatal(err)
	}
}

// ---------- defeitos corrigidos (regressão) ----------

// clean.go:129 fatia o GUID antes de validar o tamanho do chunk.
func TestQA_Regress_ShortExtendedXMPChunkPanics(t *testing.T) {
	// o slice só estoura quando faltam <32 bytes até o fim do buffer (cap), por isso o SOS vem logo depois
	in := append([]byte{0xFF, mSOI}, seg(mAPP1, append(append([]byte{}, xmpExt...), "ABC"...))...)
	in = append(in, 0xFF, mSOS, 0, 8)
	if _, _, _, p := noPanic(t, in); p != nil {
		t.Fatalf("panic com arquivo de %d bytes: %v", len(in), p)
	}
}

// clean.go:137 make([]byte,total) com total vindo do arquivo (até 4 GiB).
func TestQA_Regress_ExtendedXMPTotalAllocation(t *testing.T) {
	const declared = 64 << 20
	in := build(mainXMP("00000000000000000000000000000000"),
		extChunk("00000000000000000000000000000000", declared, 0, []byte("<x/>")))
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	_, _, _, p := noPanic(t, in)
	runtime.ReadMemStats(&b)
	alloc := b.TotalAlloc - a.TotalAlloc
	t.Logf("arquivo de %d bytes alocou %d MiB (total declarado %d MiB); com 0xFFFFFFFF seriam >8 GiB", len(in), alloc>>20, declared>>20)
	if p != nil || alloc > 16<<20 {
		t.Fatalf("alocação controlada pelo arquivo: %d MiB (panic=%v)", alloc>>20, p)
	}
}

// JUMBF que não é C2PA (ex.: JPEG XT / JPEG 360) também é apagado.
func TestQA_Regress_NonC2PAJumbfRemoved(t *testing.T) {
	box := []byte{0, 0, 0, 0x2A, 'j', 'u', 'm', 'b', 0, 0, 0, 0x22, 'j', 'u', 'm', 'd'}
	box = append(box, bytes.Repeat([]byte{0x11}, 16)...) // UUID de tipo != c2pa
	box = append(box, 0x03, 'j', 'p', 'x', 't', 0, 0)
	app := seg(mAPP11, append([]byte{'J', 'P', 0, 5, 0, 0, 0, 1}, box...))
	in := build(app)
	out, _, err := Clean(in)
	if err != nil || !bytes.Contains(out, app) {
		t.Fatal("APP11/JUMBF sem rótulo c2pa foi removido")
	}
}

// Palavra-chave legítima "firefly" (ex.: foto de vaga-lumes) vira "PERSISTE".
func TestQA_Regress_FireflyKeywordFalsePositive(t *testing.T) {
	x := `<rdf:Description xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:subject><rdf:Bag><rdf:li>firefly</rdf:li></rdf:Bag></dc:subject></rdf:Description>`
	in := build(seg(mAPP1, append(append([]byte{}, xmpStd...), x...)))
	_, info, err := Clean(in)
	if err != nil || len(info.Found) > 0 || len(info.Remaining) > 0 {
		t.Fatalf("falso positivo: %+v", info)
	}
}

// APP2/MPF antes do APP11: remover o APP11 desloca a 2ª imagem e o offset do MPF fica errado.
func TestQA_Regress_MPFOffsetAfterRemoval(t *testing.T) {
	be := binary.BigEndian
	tiff := []byte{'M', 'M', 0, 0x2A, 0, 0, 0, 8}
	ifd := make([]byte, 2+3*12+4)
	be.PutUint16(ifd[0:], 3)
	e := ifd[2:]
	be.PutUint16(e[0:], 0xB000)
	be.PutUint16(e[2:], 7)
	be.PutUint32(e[4:], 4)
	copy(e[8:], "0100")
	be.PutUint16(e[12:], 0xB001)
	be.PutUint16(e[14:], 4)
	be.PutUint32(e[16:], 1)
	be.PutUint32(e[20:], 2)
	be.PutUint16(e[24:], 0xB002)
	be.PutUint16(e[26:], 7)
	be.PutUint32(e[28:], 32)
	be.PutUint32(e[32:], 50)
	entries := make([]byte, 32)
	mpf := append(append(append([]byte("MPF\x00"), tiff...), ifd...), entries...)
	second := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	mk := func() []byte {
		first := build(seg(mAPP2, mpf), jumb(1, 1))
		return append(first, second...)
	}
	in := mk()
	const tiffStart = 2 + 4 + 4
	be.PutUint32(mpf[4+50+16+8:], uint32(len(in)-len(second)-tiffStart))
	in = mk()
	if !bytes.Equal(in[tiffStart+int(be.Uint32(in[tiffStart+50+16+8:])):][:2], []byte{0xFF, 0xD8}) {
		t.Fatal("montagem do teste")
	}
	out, _, err := Clean(in)
	if err != nil {
		t.Fatal(err)
	}
	off := int(be.Uint32(out[tiffStart+50+16+8:]))
	if tiffStart+off+2 > len(out) || !bytes.Equal(out[tiffStart+off:tiffStart+off+2], []byte{0xFF, 0xD8}) {
		t.Fatalf("offset MPF da 2ª imagem (%d) não aponta para SOI após remover %d bytes de APP11", off, len(in)-len(out))
	}
}

// ---------- fuzz ----------

// skipKnown evita que o fuzz pare nos dois defeitos já conhecidos do XMP
// estendido (para procurar outros). Ativo só com LIMPAIA_FUZZ_SKIP_KNOWN=1.
func skipKnown(data []byte) bool {
	if os.Getenv("LIMPAIA_FUZZ_SKIP_KNOWN") == "" {
		return false
	}
	segs, _, err := Parse(data)
	if err != nil {
		return false
	}
	for _, s := range segs {
		if s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpExt) {
			if len(s.Payload) < len(xmpExt)+40 || binary.BigEndian.Uint32(s.Payload[len(xmpExt)+32:]) > 1<<20 {
				return true
			}
		}
	}
	return false
}

func FuzzClean(f *testing.F) {
	content := extContent(5)
	f.Add(build(seg(mAPP1, []byte("Exif\x00\x00zz"))))
	f.Add(build(jumb(1, 1), jumb(1, 2), seg(mAPP11, []byte("XXnot-jumbf"))))
	f.Add(build(append([][]byte{mainXMP(guidOf(content))}, extSegments(content, 200)...)...))
	f.Add(build(seg(mAPP1, append(append([]byte{}, xmpStd...), `<a Iptc4xmpExt:DigitalSourceType="http://cv.iptc.org/newscodes/digitalsourcetype/trainedAlgorithmicMedia" crs:fill_method="firefly"/>`...))))
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xFF, 0xFF, 0xDA})
	f.Fuzz(func(t *testing.T, data []byte) {
		if skipKnown(data) {
			t.Skip()
		}
		out, _, err := Clean(data)
		if err != nil {
			return
		}
		if _, _, err := Parse(out); err != nil {
			t.Fatalf("saída não reparseável: %v", err)
		}
		if err := PreservedEqual(data, out); err != nil {
			t.Fatalf("PreservedEqual: %v", err)
		}
		Dimensions(out)
	})
}
