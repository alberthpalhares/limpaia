# Gera os pacotes em dist\: LimpaIA.exe (Windows), LimpaIA-macOS.zip (Mac universal: chip Apple + Intel)
# e LimpaIA.html (versão web: abre no navegador, sem instalar)
$ErrorActionPreference = 'Stop'; Set-Location $PSScriptRoot
function Run($what) { & $what; if ($LASTEXITCODE) { Write-Error "falhou: $what"; exit 1 } }
Run { go vet ./... }
$env:GOOS='darwin'; Run { go vet ./cmd/... }; $env:GOOS='linux'; Run { go vet ./cmd/... }; Remove-Item Env:GOOS
$env:GOOS='js'; $env:GOARCH='wasm'; Run { go vet ./cmd/limpaia-web }; Remove-Item Env:GOOS, Env:GOARCH
Run { go test ./... }
New-Item -ItemType Directory -Force dist, build | Out-Null
$env:CGO_ENABLED = '0'
$flags = @('-trimpath', '-ldflags', '-s -w')
$env:GOOS='windows'; $env:GOARCH='amd64'; Run { go build @flags -o dist\LimpaIA.exe ./cmd/limpaia }
$env:GOOS='darwin';  $env:GOARCH='arm64'; Run { go build @flags -o build\LimpaIA-mac-arm64 ./cmd/limpaia }
$env:GOARCH='amd64'; Run { go build @flags -o build\LimpaIA-mac-intel ./cmd/limpaia }
$env:GOOS='js';      $env:GOARCH='wasm';  Run { go build @flags -o build\limpaia.wasm ./cmd/limpaia-web }
Remove-Item Env:GOOS, Env:GOARCH
Run { go run ./tools/mkmac -arm64 build\LimpaIA-mac-arm64 -amd64 build\LimpaIA-mac-intel -leiame packaging\LEIA-ME_macOS.txt -out dist\LimpaIA-macOS.zip }
Run { go run ./tools/mkweb -wasm build\limpaia.wasm -modelo web\index.html -out dist\LimpaIA.html }
Copy-Item packaging\LEIA-ME_macOS.txt, packaging\LEIA-ME_Windows.txt, packaging\LEIA-ME_Web.txt dist\ -Force
