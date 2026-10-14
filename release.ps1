# Publica uma versão no GitHub Releases:  .\release.ps1 -Versao 1.0.0
# Gera os pacotes (build.ps1), os instaladores de uma linha (tools\mkinstall, com
# a versão e os SHA-256 embutidos), cria a tag e a release com todos os arquivos.
param([Parameter(Mandatory)][ValidatePattern('^\d+\.\d+\.\d+$')][string]$Versao)
$ErrorActionPreference = 'Stop'; Set-Location $PSScriptRoot
function Run($what) { & $what; if ($LASTEXITCODE) { Write-Error "falhou: $what"; exit 1 } }
$repo = 'alberthpalhares/limpaia'
$tag = "v$Versao"

if (git status --porcelain) { Write-Error 'há alterações sem commit; faça o commit antes de publicar'; exit 1 }
if (git tag -l $tag) { Write-Error "a versão $tag já existe"; exit 1 }

Run { .\build.ps1 }
Run { go run ./tools/mkinstall -repo $repo -tag $tag -mac dist\LimpaIA-macOS.zip -win dist\LimpaIA.exe -out dist }

$notes = @'
## Instalar com um comando (sem aviso do Gatekeeper/SmartScreen)

**Mac** (app Terminal):
```
curl -fsSL https://github.com/REPO/releases/latest/download/install.sh | sh
```

**Windows** (PowerShell):
```
irm https://github.com/REPO/releases/latest/download/install.ps1 | iex
```

**Sem instalar** (qualquer sistema): baixe `LimpaIA.html` e abra no navegador.

Os instaladores conferem o SHA-256 de cada pacote (veja `SHA256SUMS.txt`).
'@ -replace 'REPO', $repo

Run { git push origin HEAD }
Run { git tag -a $tag -m "LimpaIA $tag" }
Run { git push origin $tag }
$assets = 'LimpaIA-macOS.zip', 'LimpaIA.exe', 'LimpaIA.html', 'install.sh', 'install.ps1', 'SHA256SUMS.txt',
	'LEIA-ME_macOS.txt', 'LEIA-ME_Windows.txt', 'LEIA-ME_Web.txt' | ForEach-Object { "dist\$_" }
Run { gh release create $tag --repo $repo --title "LimpaIA $tag" --notes $notes @assets }
