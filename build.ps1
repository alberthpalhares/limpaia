# Copyright 2026 Alberth Palhares · SPDX-License-Identifier: Apache-2.0
# Gera os pacotes em dist\: LimpaIA.exe (Windows), LimpaIA-macOS.zip (Mac universal: chip Apple + Intel)
# e LimpaIA.html (versão web: abre no navegador, sem instalar).  Uso: .\build.ps1 [-Versao 1.2.3]
param([ValidatePattern('^\d+\.\d+\.\d+$')][string]$Versao = '0.0.0')
$ErrorActionPreference = 'Stop'; Set-Location $PSScriptRoot
function Run($what) { & $what; if ($LASTEXITCODE) { Write-Error "falhou: $what"; exit 1 } }
$tag = "v$Versao"
Run { go vet ./... }
$env:GOOS='darwin'; Run { go vet ./cmd/... }; $env:GOOS='linux'; Run { go vet ./cmd/... }; Remove-Item Env:GOOS
$env:GOOS='js'; $env:GOARCH='wasm'; Run { go vet ./cmd/limpaia-web }; Remove-Item Env:GOOS, Env:GOARCH
Run { go test ./... }
New-Item -ItemType Directory -Force dist, build | Out-Null

# ícone, versão e copyright dentro do .exe (recurso do Windows; gerado a cada build porque leva a versão)
Run { go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out cmd\limpaia\rsrc --icon assets\icon\LimpaIA.ico `
	--product-name LimpaIA --file-description 'LimpaIA - remove o selo de IA (C2PA) de fotos' `
	--copyright 'Copyright 2026 Alberth Palhares - Apache-2.0' --original-filename LimpaIA.exe `
	--product-version "$Versao.0" --file-version "$Versao.0" }

$env:CGO_ENABLED = '0'
$flags = @('-trimpath', '-ldflags', "-s -w -X main.version=$tag")
$env:GOOS='windows'; $env:GOARCH='amd64'; Run { go build @flags -o dist\LimpaIA.exe ./cmd/limpaia }
$env:GOOS='darwin';  $env:GOARCH='arm64'; Run { go build @flags -o build\LimpaIA-mac-arm64 ./cmd/limpaia }
$env:GOARCH='amd64'; Run { go build @flags -o build\LimpaIA-mac-intel ./cmd/limpaia }
$env:GOOS='js';      $env:GOARCH='wasm';  Run { go build @flags -o build\limpaia.wasm ./cmd/limpaia-web }
Remove-Item Env:GOOS, Env:GOARCH
Run { go run ./tools/mkmac -arm64 build\LimpaIA-mac-arm64 -amd64 build\LimpaIA-mac-intel -icns assets\icon\LimpaIA.icns -versao $Versao -leiame packaging\LEIA-ME_macOS.txt -out dist\LimpaIA-macOS.zip }
Run { go run ./tools/mkweb -wasm build\limpaia.wasm -modelo web\index.html -versao $tag -out dist\LimpaIA.html }
Copy-Item packaging\LEIA-ME_macOS.txt, packaging\LEIA-ME_Windows.txt, packaging\LEIA-ME_Web.txt, NOTICE dist\ -Force
