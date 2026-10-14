package jpegclean

import (
	"bytes"
	"testing"
)

// dcterms:provenance aponta para o manifest C2PA (inclusive remoto): é removido.
func TestQA_Regress_ProvenanceRemoved(t *testing.T) {
	for _, v := range []string{"urn:c2pa:1234-abcd", "https://cai-manifests.adobe.com/manifests/abc"} {
		x := `<rdf:Description xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dc="http://purl.org/dc/elements/1.1/" dcterms:provenance="` + v + `" dc:rights="© Fulano"/>`
		in := build(seg(mAPP1, append(append([]byte{}, xmpStd...), x...)))
		out, info, err := Clean(in)
		if err != nil || !info.Changed || len(info.Remaining) != 0 {
			t.Fatalf("%s: err=%v info=%+v", v, err, info)
		}
		if bytes.Contains(out, []byte("provenance")) || !bytes.Contains(out, []byte("© Fulano")) {
			t.Fatalf("%s: provenance deveria sumir e o copyright ficar", v)
		}
	}
	// provenance sem relação com C2PA fica intacto
	x := `<rdf:Description xmlns:dcterms="http://purl.org/dc/terms/" dcterms:provenance="Acervo da família"/>`
	in := build(seg(mAPP1, append(append([]byte{}, xmpStd...), x...)))
	if out, _, err := Clean(in); err != nil || !bytes.Equal(out, in) {
		t.Fatal("provenance comum foi alterado")
	}
}

// Blocos de XMP estendido com outro GUID (restos de salvamentos anteriores)
// não podem manter a marca "firefly" no arquivo.
func TestQA_Regress_OrphanExtendedXMP(t *testing.T) {
	cur, old := extContent(4), extContent(2)
	parts := [][]byte{mainXMP(guidOf(cur))}
	parts = append(parts, extSegments(old, 200)...)
	parts = append(parts, extSegments(cur, 200)...)
	in := build(parts...)
	out, info, err := Clean(in)
	if err != nil || !info.Changed || len(info.Remaining) != 0 {
		t.Fatalf("err=%v info=%+v", err, info)
	}
	if bytes.Contains(out, []byte("firefly")) {
		t.Fatal("marca firefly continua no arquivo")
	}
	if err := PreservedEqual(in, out); err != nil {
		t.Fatal(err)
	}
	_, content := reassemble(t, out)
	wellFormed(t, content)
}
