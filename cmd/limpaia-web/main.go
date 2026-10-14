//go:build js && wasm

// limpaia-web expõe a limpeza para a página web (dist/LimpaIA.html, gerada por
// tools/mkweb). Funções globais em JS:
//
//	limpaiaProcess(nome, Uint8Array) -> {out: Uint8Array|null, result: JSON de FileResult}
//	limpaiaReport(JSON de []FileResult) -> {base, txt (sem BOM), csv: Uint8Array com BOM, summary}
package main

import (
	"encoding/json"
	"syscall/js"
	"time"

	"limpaia/internal/runner"
)

func bytesToJS(b []byte) js.Value {
	u := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(u, b)
	return u
}

func process(_ js.Value, args []js.Value) any {
	data := make([]byte, args[1].Get("length").Int())
	js.CopyBytesToGo(data, args[1])
	out, fr := runner.Process(args[0].String(), data)
	res, _ := json.Marshal(fr)
	o := js.Null()
	if out != nil {
		o = bytesToJS(out)
	}
	return js.ValueOf(map[string]any{"out": o, "result": string(res)})
}

func report(_ js.Value, args []js.Value) any {
	var s runner.Summary
	if err := json.Unmarshal([]byte(args[0].String()), &s.Results); err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}
	s.OutDirs = []string{runner.OutDirName}
	now := time.Now()
	return js.ValueOf(map[string]any{
		"base":    "RELATORIO_" + now.Format("20060102_150405"),
		"txt":     runner.ReportText(s, now), // sem BOM: o TextDecoder do JS o descartaria
		"csv":     bytesToJS(runner.ReportCSV(s)),
		"summary": s.Text(),
	})
}

func main() {
	js.Global().Set("limpaiaProcess", js.FuncOf(process))
	js.Global().Set("limpaiaReport", js.FuncOf(report))
	if f := js.Global().Get("onLimpaiaReady"); f.Type() == js.TypeFunction {
		f.Invoke()
	}
	select {}
}
