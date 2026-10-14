// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// mkicon desenha o ícone do LimpaIA (só biblioteca padrão: formas geométricas
// com antialiasing por supersampling) e grava em assets/icon/:
// icon-<N>.png, LimpaIA.ico (Windows) e LimpaIA.icns (macOS).
//
// Desenho: quadrado arredondado azul, uma foto branca (montanha e sol) e um
// selo vermelho com − — "tira a marca de IA da foto".
//
// Uso: go run ./tools/mkicon [-out assets/icon]
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
}

type rgba struct{ r, g, b, a float64 }

func hex(h uint32) rgba {
	return rgba{float64(h>>16&255) / 255, float64(h>>8&255) / 255, float64(h&255) / 255, 1}
}

func roundRect(u, v, x0, y0, x1, y1, r float64) bool {
	dx := math.Max(math.Max(x0+r-u, u-(x1-r)), 0)
	dy := math.Max(math.Max(y0+r-v, v-(y1-r)), 0)
	return u >= x0 && u <= x1 && v >= y0 && v <= y1 && dx*dx+dy*dy <= r*r
}

func circle(u, v, cx, cy, r float64) bool { return (u-cx)*(u-cx)+(v-cy)*(v-cy) <= r*r }

// inPoly: regra par-ímpar.
func inPoly(u, v float64, p [][2]float64) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		if (p[i][1] > v) != (p[j][1] > v) && u < (p[j][0]-p[i][0])*(v-p[i][1])/(p[j][1]-p[i][1])+p[i][0] {
			in = !in
		}
	}
	return in
}

// capsule: segmento (ax,ay)-(bx,by) com espessura w.
func capsule(u, v, ax, ay, bx, by, w float64) bool {
	px, py, dx, dy := u-ax, v-ay, bx-ax, by-ay
	t := math.Max(0, math.Min(1, (px*dx+py*dy)/(dx*dx+dy*dy)))
	ex, ey := px-t*dx, py-t*dy
	return ex*ex+ey*ey <= w*w/4
}

type design struct {
	card, inner   [4]float64 // x0 y0 x1 y1
	cardR         float64
	badge         [2]float64 // centro do selo
	badgeR        float64
	ring          float64 // borda branca do selo
	cross, crossW float64 // meio comprimento e espessura do "×"
}

var (
	big = design{
		card: [4]float64{0.13, 0.22, 0.79, 0.74}, inner: [4]float64{0.185, 0.275, 0.735, 0.685}, cardR: 0.07,
		badge: [2]float64{0.735, 0.715}, badgeR: 0.19, ring: 0.045, cross: 0.085, crossW: 0.06,
	}
	small = design{ // 16–24 px: selo maior e traços mais grossos, sem o sol
		card: [4]float64{0.06, 0.14, 0.80, 0.74}, inner: [4]float64{0.14, 0.22, 0.72, 0.66}, cardR: 0.08,
		badge: [2]float64{0.68, 0.68}, badgeR: 0.235, ring: 0.05, cross: 0.105, crossW: 0.095,
	}
)

// palette: cores e símbolo do selo. As variantes servem só para comparar
// opções (-variante); a padrão é a escolhida para o projeto.
type palette struct {
	bgTop, bgBot, card, sky, mountain, badge uint32
	glyph                                    string // "x", "check" ou "minus"
}

var variants = map[string]palette{
	"a": {0x3B82F6, 0x1E40AF, 0xFFFFFF, 0xDBEAFE, 0x2563EB, 0xEF4444, "x"},     // azul + × vermelho
	"b": {0x3B82F6, 0x1E40AF, 0xFFFFFF, 0xDBEAFE, 0x2563EB, 0x16A34A, "check"}, // azul + ✓ verde
	"c": {0x3B82F6, 0x1E40AF, 0xFFFFFF, 0xDBEAFE, 0x2563EB, 0xEF4444, "minus"}, // azul + − vermelho
	"d": {0xFFFFFF, 0xE5E7EB, 0x2563EB, 0xBFDBFE, 0x1D4ED8, 0xEF4444, "x"},     // claro + × vermelho
	"e": {0x334155, 0x0F172A, 0xFFFFFF, 0xE2E8F0, 0x475569, 0x22C55E, "check"}, // grafite + ✓ verde
}

var pal = variants["c"]

// sample devolve a cor no ponto (u,v) do quadrado unitário (camadas de baixo para cima).
func (d design) sample(u, v float64, detail bool) rgba {
	if !roundRect(u, v, 0, 0, 1, 1, 0.225) {
		return rgba{}
	}
	top, bot := hex(pal.bgTop), hex(pal.bgBot)
	c := rgba{top.r + (bot.r-top.r)*v, top.g + (bot.g-top.g)*v, top.b + (bot.b-top.b)*v, 1}

	k := d.card
	if roundRect(u, v, k[0], k[1], k[2], k[3], d.cardR) {
		c = hex(pal.card)
		in := d.inner
		if u >= in[0] && u <= in[2] && v >= in[1] && v <= in[3] {
			c = hex(pal.sky) // céu
			w, h := in[2]-in[0], in[3]-in[1]
			x := func(f float64) float64 { return in[0] + f*w }
			y := func(f float64) float64 { return in[1] + f*h }
			if detail && circle(u, v, x(0.27), y(0.30), 0.10*w) {
				c = hex(0xFBBF24) // sol
			}
			if inPoly(u, v, [][2]float64{{x(-0.05), y(1)}, {x(0.42), y(0.38)}, {x(0.95), y(1)}}) {
				c = hex(pal.mountain) // montanha
			}
		}
	}

	// selo: círculo com borda branca e o símbolo
	bx, by := d.badge[0], d.badge[1]
	if circle(u, v, bx, by, d.badgeR+d.ring) {
		c = hex(0xFFFFFF)
	}
	if circle(u, v, bx, by, d.badgeR) {
		c = hex(pal.badge)
		r, w := d.cross, d.crossW
		var mark bool
		switch pal.glyph {
		case "check":
			mark = capsule(u, v, bx-r, by+0.05*r, bx-0.25*r, by+0.75*r, w) || capsule(u, v, bx-0.25*r, by+0.75*r, bx+r, by-0.65*r, w)
		case "minus":
			mark = capsule(u, v, bx-r, by, bx+r, by, w*1.1)
		default:
			mark = capsule(u, v, bx-r, by-r, bx+r, by+r, w) || capsule(u, v, bx-r, by+r, bx+r, by-r, w)
		}
		if mark {
			c = hex(0xFFFFFF)
		}
	}
	return c
}

func render(n int) *image.NRGBA {
	d, detail, ss := big, true, 4
	if n <= 24 {
		d, detail, ss = small, false, 8
	} else if n <= 64 {
		ss = 6
	}
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					c := d.sample((float64(px)+(float64(sx)+0.5)/float64(ss))/float64(n), (float64(py)+(float64(sy)+0.5)/float64(ss))/float64(n), detail)
					r, g, b, a = r+c.r*c.a, g+c.g*c.a, b+c.b*c.a, a+c.a
				}
			}
			if a == 0 {
				continue
			}
			k := float64(ss * ss)
			img.SetNRGBA(px, py, color.NRGBA{uint8(r/a*255 + 0.5), uint8(g/a*255 + 0.5), uint8(b/a*255 + 0.5), uint8(a/k*255 + 0.5)})
		}
	}
	return img
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	check((&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, img))
	return b.Bytes()
}

// ico: entradas PNG (aceitas desde o Windows Vista).
func ico(pngs map[int][]byte, sizes []int) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	w(uint16(0))
	w(uint16(1))
	w(uint16(len(sizes)))
	off := 6 + 16*len(sizes)
	for _, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0
		}
		w(dim)
		w(dim)
		w(uint8(0))
		w(uint8(0))
		w(uint16(1))
		w(uint16(32))
		w(uint32(len(pngs[s])))
		w(uint32(off))
		off += len(pngs[s])
	}
	for _, s := range sizes {
		b.Write(pngs[s])
	}
	return b.Bytes()
}

// icns: contêiner de PNGs (tipos para 1x e 2x).
func icns(pngs map[int][]byte) []byte {
	types := []struct {
		t string
		s int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024},
		{"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512}}
	var body bytes.Buffer
	for _, e := range types {
		body.WriteString(e.t)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pngs[e.s])))
		body.Write(pngs[e.s])
	}
	var b bytes.Buffer
	b.WriteString("icns")
	binary.Write(&b, binary.BigEndian, uint32(8+body.Len()))
	b.Write(body.Bytes())
	return b.Bytes()
}

func main() {
	out := flag.String("out", filepath.Join("assets", "icon"), "pasta de saída")
	v := flag.String("variante", "c", "variante de cores/símbolo (a–e), para comparar opções")
	embed := flag.String("embed", filepath.Join("cmd", "limpaia", "LimpaIA.ico"), "cópia do .ico usada pelo go:embed (vazio: não grava)")
	flag.Parse()
	p, ok := variants[*v]
	if !ok {
		check(fmt.Errorf("variante %q desconhecida", *v))
	}
	pal = p
	check(os.MkdirAll(*out, 0o755))
	pngs := map[int][]byte{}
	for _, n := range []int{16, 24, 32, 48, 64, 128, 256, 512, 1024} {
		pngs[n] = encode(render(n))
		check(os.WriteFile(filepath.Join(*out, fmt.Sprintf("icon-%d.png", n)), pngs[n], 0o644))
	}
	icoData := ico(pngs, []int{16, 24, 32, 48, 64, 128, 256})
	check(os.WriteFile(filepath.Join(*out, "LimpaIA.ico"), icoData, 0o644))
	if *embed != "" { // cópia embutida no .exe (go:embed não alcança pastas acima do pacote)
		check(os.WriteFile(*embed, icoData, 0o644))
	}
	check(os.WriteFile(filepath.Join(*out, "LimpaIA.icns"), icns(pngs), 0o644))
	fmt.Println(*out + ": PNGs 16–1024, LimpaIA.ico e LimpaIA.icns")
}
