// Package jpegclean remove, no nível de segmentos JPEG, os indicadores de IA
// generativa (C2PA/Content Credentials e DigitalSourceType em XMP) sem
// decodificar nem recomprimir a imagem: tudo a partir do primeiro SOS é
// copiado byte a byte.
package jpegclean

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	mSOI   = 0xD8
	mSOS   = 0xDA
	mAPP1  = 0xE1
	mAPP2  = 0xE2
	mAPP11 = 0xEB
	mAPP13 = 0xED
)

var (
	xmpStd = []byte("http://ns.adobe.com/xap/1.0/\x00")
	xmpExt = []byte("http://ns.adobe.com/xmp/extension/\x00")

	// valores de DigitalSourceType que indicam mídia gerada/composta por IA
	aiValue = `(?:composite)?(?:with)?trainedalgorithmicmedia|algorithmicmedia|compositesynthetic|compositewithtrained[a-z]*`
	// propriedade como atributo: Iptc4xmpExt:DigitalSourceType="...ai..."
	reDSTAttr = regexp.MustCompile(`(?is)\s+[\w.-]+:DigitalSourceType\s*=\s*"[^"]*(?:` + aiValue + `)[^"]*"`)
	// propriedade como elemento, com texto ou rdf:resource
	reDSTElem = regexp.MustCompile(`(?is)<([\w.-]+):DigitalSourceType\b[^>]*?(?:/>|>[^<]*</[\w.-]+:DigitalSourceType\s*>)`)
	// Lightroom/Camera Raw registra remoção generativa como crs:fill_method="firefly"
	reFill = regexp.MustCompile(`(?i)\s+crs:fill_method\s*=\s*"firefly"`)
	// marcadores que, se sobrarem, indicam persistência
	// (a palavra "firefly" sozinha, p.ex. numa palavra-chave sobre vaga-lumes, não conta)
	reMarker  = regexp.MustCompile(`(?i)trainedAlgorithmicMedia|compositeWithTrained|compositeSynthetic|adobe\.firefly|fill_method\s*=\s*"firefly"|generativefill|generative fill|c2pa|cai-manifests|contentcredentials|jumbf`)
	reAIValue = regexp.MustCompile(`(?i)` + aiValue)
	// dcterms:provenance aponta para o manifest C2PA (inclusive remoto, na nuvem)
	reProv    = regexp.MustCompile(`(?is)\s+dcterms:provenance\s*=\s*"[^"]*(?:c2pa|cai-manifests|contentauth)[^"]*"`)
	reProvEl  = regexp.MustCompile(`(?is)<dcterms:provenance\b[^>]*?(?:/>|>[^<]*</dcterms:provenance\s*>)`)
	reProvVal = regexp.MustCompile(`(?i)c2pa|cai-manifests|contentauth`)
	reHasExt  = regexp.MustCompile(`HasExtendedXMP\s*=\s*"([0-9A-Fa-f]{32})"`)
)

// Limite de memória do XMP estendido reconstruído (o total declarado vem do arquivo).
const maxExtendedXMP = 64 << 20

// Segment é um segmento JPEG anterior ao SOS.
type Segment struct {
	Marker  byte
	Payload []byte // sem os 2 bytes de comprimento
	Raw     []byte // marcador + comprimento + payload
	Off     int    // posição de Raw no arquivo
}

// Info descreve o que foi encontrado/removido.
type Info struct {
	Found     []string // estruturas com indicador de IA encontradas
	Removed   []string // estruturas removidas
	Remaining []string // indicadores que persistem no resultado
	Changed   bool
}

// Parse devolve os segmentos até o SOS e o offset do SOS.
func Parse(data []byte) (segs []Segment, scanOff int, err error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != mSOI {
		return nil, 0, errors.New("não é um JPEG (SOI ausente)")
	}
	i := 2
	for {
		if i+4 > len(data) {
			return nil, 0, errors.New("JPEG truncado antes do SOS")
		}
		if data[i] != 0xFF {
			return nil, 0, fmt.Errorf("marcador inválido no offset %d", i)
		}
		m := data[i+1]
		if m == 0xFF { // preenchimento
			i++
			continue
		}
		if m == mSOS {
			return segs, i, nil
		}
		l := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if l < 2 || i+2+l > len(data) {
			return nil, 0, fmt.Errorf("segmento 0x%02X com tamanho inválido", m)
		}
		segs = append(segs, Segment{Marker: m, Payload: data[i+4 : i+2+l], Raw: data[i : i+2+l], Off: i})
		i += 2 + l
	}
}

func isJUMBF(p []byte) bool { return len(p) >= 2 && p[0] == 'J' && p[1] == 'P' }

// c2paInstances devolve as instâncias APP11/JUMBF (En) que são manifests C2PA:
// o UUID/rótulo "c2pa" (ou o antigo "cai") aparece logo no início de algum
// fragmento. JUMBF de outros usos (JPEG XT, JPEG 360...) não é tocado.
func c2paInstances(segs []Segment) map[uint16]bool {
	r := map[uint16]bool{}
	for _, s := range segs {
		if s.Marker != mAPP11 || !isJUMBF(s.Payload) {
			continue
		}
		head := s.Payload
		if len(head) > 96 {
			head = head[:96]
		}
		if bytes.Contains(head, []byte("c2pa")) || bytes.Contains(head, []byte("cai\x00")) {
			r[instance(s.Payload)] = true
		}
	}
	return r
}

// instance devolve o "Box Instance Number" (En) de um APP11 JUMBF.
func instance(p []byte) uint16 {
	if len(p) < 4 {
		return 0
	}
	return binary.BigEndian.Uint16(p[2:4])
}

// stripAI remove do texto XMP os indicadores de IA conhecidos.
func stripAI(s string) string {
	// prefiltro: evita rodar as regex em XMP grande que não menciona nada disso
	if l := strings.ToLower(s); !strings.Contains(l, "digitalsourcetype") && !strings.Contains(l, "fill_method") && !strings.Contains(l, "provenance") {
		return s
	}
	n := reFill.ReplaceAllString(s, "")
	n = reDSTAttr.ReplaceAllString(n, "")
	n = reProv.ReplaceAllString(n, "")
	n = reProvEl.ReplaceAllStringFunc(n, func(m string) string {
		if reProvVal.MatchString(m) {
			return ""
		}
		return m
	})
	return reDSTElem.ReplaceAllStringFunc(n, func(m string) string {
		if reAIValue.MatchString(m) {
			return ""
		}
		return m
	})
}

func cleanXMP(payload []byte) (out []byte, removed bool) {
	head := xmpStd
	s := string(payload[len(head):])
	n := stripAI(s)
	if n == s {
		return payload, false
	}
	return append(append([]byte{}, head...), n...), true
}

// cleanExtended reúne e limpa o XMP estendido (vários APP1 com GUID/offset).
// Devolve: guid antigo, novo guid, novos segmentos e se houve mudança.
// Qualquer inconsistência (cabeçalho curto, GUIDs diferentes, remendos faltando
// ou sobrepostos, total declarado maior que o conteúdo) deixa o XMP intacto.
func cleanExtended(segs []Segment, mainGUID string) (oldGUID, newGUID string, newSegs [][]byte, changed bool) {
	hl := len(xmpExt) + 32 + 8
	var chunks []Segment
	for _, s := range segs {
		if s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpExt) {
			if mainGUID != "" && len(s.Payload) >= len(xmpExt)+32 && string(s.Payload[len(xmpExt):len(xmpExt)+32]) != mainGUID {
				continue // bloco órfão (de uma versão anterior do XMP): tratado em Clean
			}
			chunks = append(chunks, s)
		}
	}
	if len(chunks) == 0 {
		return
	}
	type piece struct {
		off  int
		data []byte
	}
	var pieces []piece
	sum := 0
	total := 0
	for i, c := range chunks {
		if len(c.Payload) < hl {
			return "", "", nil, false
		}
		g := string(c.Payload[len(xmpExt) : len(xmpExt)+32])
		if i == 0 {
			oldGUID = g
		} else if g != oldGUID {
			return "", "", nil, false
		}
		total = int(binary.BigEndian.Uint32(c.Payload[len(xmpExt)+32:]))
		pieces = append(pieces, piece{int(binary.BigEndian.Uint32(c.Payload[len(xmpExt)+36:])), c.Payload[hl:]})
		sum += len(c.Payload) - hl
	}
	if total != sum || total > maxExtendedXMP {
		return "", "", nil, false
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].off < pieces[j].off })
	buf := make([]byte, 0, total)
	for _, p := range pieces {
		if p.off != len(buf) { // lacuna ou sobreposição
			return "", "", nil, false
		}
		buf = append(buf, p.data...)
	}
	n := stripAI(string(buf))
	if n == string(buf) {
		return oldGUID, oldGUID, nil, false
	}
	sumMD5 := md5.Sum([]byte(n))
	newGUID = strings.ToUpper(hex.EncodeToString(sumMD5[:]))
	const maxData = 65000
	for off := 0; off < len(n); off += maxData {
		end := off + maxData
		if end > len(n) {
			end = len(n)
		}
		p := append([]byte{}, xmpExt...)
		p = append(p, newGUID...)
		var w [8]byte
		binary.BigEndian.PutUint32(w[0:], uint32(len(n)))
		binary.BigEndian.PutUint32(w[4:], uint32(off))
		p = append(p, w[:]...)
		p = append(p, n[off:end]...)
		l := len(p) + 2
		newSegs = append(newSegs, append([]byte{0xFF, mAPP1, byte(l >> 8), byte(l)}, p...))
	}
	return oldGUID, newGUID, newSegs, true
}

// Clean remove os indicadores de IA e devolve o novo arquivo.
func Clean(data []byte) ([]byte, Info, error) {
	var info Info
	segs, scanOff, err := Parse(data)
	if err != nil {
		return nil, info, err
	}

	// instâncias JUMBF de manifest C2PA (fragmentos APP11 do mesmo manifest compartilham En)
	drop := c2paInstances(segs)

	// GUID do XMP estendido referenciado pelo XMP principal; blocos com outro GUID
	// são órfãos (restos de salvamentos anteriores) e, pela especificação, ignorados.
	mainGUID := ""
	for _, s := range segs {
		if s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpStd) {
			if m := reHasExt.FindSubmatch(s.Payload); m != nil {
				mainGUID = strings.ToUpper(string(m[1]))
			}
		}
	}
	isOrphan := func(s Segment) bool {
		return mainGUID != "" && s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpExt) &&
			len(s.Payload) >= len(xmpExt)+32 && !strings.EqualFold(string(s.Payload[len(xmpExt):len(xmpExt)+32]), mainGUID)
	}
	oldGUID, newGUID, extSegs, extChanged := cleanExtended(segs, mainGUID)
	if extChanged {
		info.Changed = true
		info.Found = append(info.Found, "XMP estendido: indicador de IA")
		info.Removed = append(info.Removed, "XMP estendido: crs:fill_method=firefly")
	}
	extDone := false

	var out bytes.Buffer
	out.Grow(len(data))
	out.Write([]byte{0xFF, mSOI})
	nJUMB := 0
	mpfOut, mpfOrig := -1, -1 // posição do segmento MPF (APP2) na saída e no original
	for _, s := range segs {
		if mpfOut < 0 && isMPF(s) {
			mpfOut, mpfOrig = out.Len(), s.Off
		}
		switch {
		case s.Marker == mAPP11 && isJUMBF(s.Payload) && drop[instance(s.Payload)]:
			nJUMB++
			info.Changed = true
			continue
		case isOrphan(s):
			info.Changed = true
			info.Removed = append(info.Removed, "XMP estendido órfão (GUID antigo)")
			continue
		case s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpExt) && extChanged:
			if !extDone {
				extDone = true
				for _, e := range extSegs {
					out.Write(e)
				}
			}
			continue
		case s.Marker == mAPP1 && bytes.HasPrefix(s.Payload, xmpStd):
			if extChanged {
				s.Payload = bytes.ReplaceAll(s.Payload, []byte(oldGUID), []byte(newGUID))
				info.Changed = true
			}
			if reMarker.Match(s.Payload) {
				info.Found = append(info.Found, "XMP: indicador de IA")
			}
			if np, ok := cleanXMP(s.Payload); ok || extChanged {
				info.Changed = true
				if ok {
					info.Removed = append(info.Removed, "XMP: DigitalSourceType de IA / crs:fill_method=firefly")
				}
				l := len(np) + 2
				if l > 0xFFFF {
					return nil, info, errors.New("XMP excede o tamanho de segmento")
				}
				out.Write([]byte{0xFF, mAPP1, byte(l >> 8), byte(l)})
				out.Write(np)
				continue
			}
		}
		out.Write(s.Raw)
	}
	if nJUMB > 0 {
		n := fmt.Sprintf("APP11/JUMBF (C2PA / Content Credentials): %d segmento(s)", nJUMB)
		info.Found = append([]string{n}, info.Found...)
		info.Removed = append([]string{n}, info.Removed...)
	}
	shift := scanOff - out.Len() // bytes removidos antes do scan (>0 quando encolheu)
	out.Write(data[scanOff:])    // dados da imagem: byte a byte

	res := out.Bytes()
	if mpfOut >= 0 && shift != 0 {
		fixMPF(res, mpfOut, mpfOrig, scanOff, shift)
	}

	// reexame do resultado
	rs, _, err := Parse(res)
	if err != nil {
		return nil, info, fmt.Errorf("resultado inválido: %w", err)
	}
	left := c2paInstances(rs)
	for _, s := range rs {
		if s.Marker == mAPP11 && isJUMBF(s.Payload) && left[instance(s.Payload)] {
			info.Remaining = append(info.Remaining, "APP11/JUMBF")
		}
		if s.Marker == mAPP1 && (bytes.HasPrefix(s.Payload, xmpStd) || bytes.HasPrefix(s.Payload, xmpExt)) && reMarker.Match(s.Payload) {
			info.Remaining = append(info.Remaining, "XMP contém marcador de IA")
		}
	}
	return res, info, nil
}

func isMPF(s Segment) bool { return s.Marker == mAPP2 && bytes.HasPrefix(s.Payload, []byte("MPF\x00")) }

// fixMPF corrige o APP2/MPF (Multi-Picture Format) quando bytes foram removidos
// antes do scan: os offsets das imagens seguintes são relativos ao cabeçalho
// MPF e o tamanho da 1ª imagem encolheu em shift bytes. res é a saída (já com o
// scan); mpfOut/mpfOrig são a posição do segmento MPF na saída/original e
// scanOff o início do scan no original. Qualquer inconsistência deixa tudo como está.
func fixMPF(res []byte, mpfOut, mpfOrig, scanOff, shift int) {
	if mpfOut+8 > len(res) {
		return
	}
	tiff := mpfOut + 8 // após FF E2 LL LL "MPF\x00"
	if tiff+8 > len(res) {
		return
	}
	var bo binary.ByteOrder
	switch string(res[tiff : tiff+2]) {
	case "MM":
		bo = binary.BigEndian
	case "II":
		bo = binary.LittleEndian
	default:
		return
	}
	segLen := int(binary.BigEndian.Uint16(res[mpfOut+2:])) + 2
	end := mpfOut + segLen
	if end > len(res) || bo.Uint16(res[tiff+2:]) != 0x2A {
		return
	}
	ifd := tiff + int(bo.Uint32(res[tiff+4:]))
	if ifd < tiff || ifd+2 > end {
		return
	}
	n := int(bo.Uint16(res[ifd:]))
	for i := 0; i < n; i++ {
		e := ifd + 2 + i*12
		if e+12 > end {
			return
		}
		if bo.Uint16(res[e:]) != 0xB002 { // MP Entry
			continue
		}
		cnt := int(bo.Uint32(res[e+4:]))
		if bo.Uint16(res[e+2:]) != 7 || cnt < 16 {
			return
		}
		val := tiff + int(bo.Uint32(res[e+8:]))
		if cnt <= 4 || val < tiff || val+cnt > end {
			return
		}
		origTiff := mpfOrig + 8
		for j := 0; j+16 <= cnt; j += 16 {
			ent := val + j
			size, off := bo.Uint32(res[ent+4:]), bo.Uint32(res[ent+8:])
			if j == 0 {
				if int(size) >= shift {
					bo.PutUint32(res[ent+4:], size-uint32(shift))
				}
				continue
			}
			if off == 0 {
				continue
			}
			abs := origTiff + int(off) // posição da imagem no arquivo original
			if abs >= scanOff && abs-shift >= tiff {
				bo.PutUint32(res[ent+8:], uint32(abs-shift-tiff))
			}
		}
		return
	}
}

// Inspect apenas diagnostica, sem gerar saída.
func Inspect(data []byte) (Info, error) {
	_, info, err := Clean(data)
	info.Remaining = nil
	return info, err
}

// Dimensions lê largura/altura do SOF.
func Dimensions(data []byte) (w, h int, err error) {
	segs, _, err := Parse(data)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range segs {
		if (s.Marker >= 0xC0 && s.Marker <= 0xCF) && s.Marker != 0xC4 && s.Marker != 0xC8 && s.Marker != 0xCC && len(s.Payload) >= 5 {
			return int(binary.BigEndian.Uint16(s.Payload[3:5])), int(binary.BigEndian.Uint16(s.Payload[1:3])), nil
		}
	}
	return 0, 0, errors.New("SOF não encontrado")
}

// PreservedEqual confere que tudo exceto os itens removidos permanece idêntico:
// segmentos não removidos (Exif, ICC, IPTC, etc.) e o scan.
func PreservedEqual(orig, clean []byte) error {
	os_, oOff, err := Parse(orig)
	if err != nil {
		return err
	}
	cs, cOff, err := Parse(clean)
	if err != nil {
		return err
	}
	if !bytes.Equal(orig[oOff:], clean[cOff:]) {
		return errors.New("dados da imagem (scan) diferem")
	}
	// segmentos que não são APP11/JUMBF nem XMP padrão devem coincidir em ordem
	c2o, c2c := c2paInstances(os_), c2paInstances(cs)
	keep := func(ss []Segment, c2 map[uint16]bool) [][]byte {
		var r [][]byte
		for _, s := range ss {
			if isMPF(s) { // offsets/tamanhos podem ser corrigidos; compara só o tamanho
				r = append(r, make([]byte, len(s.Raw)))
				continue
			}
			if (s.Marker == mAPP11 && isJUMBF(s.Payload) && c2[instance(s.Payload)]) || (s.Marker == mAPP1 && (bytes.HasPrefix(s.Payload, xmpStd) || bytes.HasPrefix(s.Payload, xmpExt))) {
				continue
			}
			r = append(r, s.Raw)
		}
		return r
	}
	a, b := keep(os_, c2o), keep(cs, c2c)
	if len(a) != len(b) {
		return errors.New("número de segmentos preservados difere")
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return fmt.Errorf("segmento preservado #%d difere", i)
		}
	}
	return nil
}
