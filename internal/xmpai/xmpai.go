// Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0

// Package xmpai reconhece e remove, do texto XMP, os indicadores de IA
// generativa (DigitalSourceType de IA, crs:fill_method="firefly" e
// dcterms:provenance apontando para C2PA). É o mesmo XMP em JPEG, PNG e WebP.
package xmpai

import (
	"regexp"
	"strings"
)

var (
	// valores de DigitalSourceType que indicam mídia gerada/composta por IA
	aiValue = `(?:composite)?(?:with)?trainedalgorithmicmedia|algorithmicmedia|compositesynthetic|compositewithtrained[a-z]*`
	// propriedade como atributo: Iptc4xmpExt:DigitalSourceType="...ai..."
	reDSTAttr = regexp.MustCompile(`(?is)\s+[\w.-]+:DigitalSourceType\s*=\s*"[^"]*(?:` + aiValue + `)[^"]*"`)
	// propriedade como elemento, com texto ou rdf:resource
	reDSTElem = regexp.MustCompile(`(?is)<([\w.-]+):DigitalSourceType\b[^>]*?(?:/>|>[^<]*</[\w.-]+:DigitalSourceType\s*>)`)
	// Lightroom/Camera Raw registra remoção generativa como crs:fill_method="firefly"
	reFill    = regexp.MustCompile(`(?i)\s+crs:fill_method\s*=\s*"firefly"`)
	reAIValue = regexp.MustCompile(`(?i)` + aiValue)
	// dcterms:provenance aponta para o manifest C2PA (inclusive remoto, na nuvem)
	reProv    = regexp.MustCompile(`(?is)\s+dcterms:provenance\s*=\s*"[^"]*(?:c2pa|cai-manifests|contentauth)[^"]*"`)
	reProvEl  = regexp.MustCompile(`(?is)<dcterms:provenance\b[^>]*?(?:/>|>[^<]*</dcterms:provenance\s*>)`)
	reProvVal = regexp.MustCompile(`(?i)c2pa|cai-manifests|contentauth`)
)

// Marker casa com os marcadores que, se sobrarem no XMP, indicam persistência
// (a palavra "firefly" sozinha, p.ex. numa palavra-chave sobre vaga-lumes, não conta).
var Marker = regexp.MustCompile(`(?i)trainedAlgorithmicMedia|compositeWithTrained|compositeSynthetic|adobe\.firefly|fill_method\s*=\s*"firefly"|generativefill|generative fill|c2pa|cai-manifests|contentcredentials|jumbf`)

// Strip remove do texto XMP os indicadores de IA conhecidos.
func Strip(s string) string {
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
