# Copyright 2026 Alberth Palhares - SPDX-License-Identifier: Apache-2.0
# Instalador do LimpaIA para Windows. Uso (no PowerShell):
#   irm https://github.com/{{REPO}}/releases/latest/download/install.ps1 | iex
#   $env:LIMPAIA_REMOVER=1; irm https://github.com/{{REPO}}/releases/latest/download/install.ps1 | iex
#
# Baixado pelo PowerShell, o programa não recebe a marca de "baixado da internet",
# então o SmartScreen não interrompe. O arquivo é conferido pelo SHA-256 antes de
# instalar. Instala só para o usuário atual (%LOCALAPPDATA%\LimpaIA), sem administrador.
# Gerado por tools/mkinstall a partir de packaging/install.ps1.
# Sem "exit": rodando via iex, fecharia a janela do PowerShell.
& {
	$ErrorActionPreference = 'Stop'
	$ProgressPreference = 'SilentlyContinue' # a barra de progresso deixa o download do PowerShell 5.1 muito lento
	[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

	$tag = '{{TAG}}'
	$sha256 = '{{SHA256_WIN}}'
	$base = if ($env:LIMPAIA_BASE_URL) { $env:LIMPAIA_BASE_URL } else { 'https://github.com/{{REPO}}/releases/download/' + $tag }
	$installed = Join-Path $env:LOCALAPPDATA 'LimpaIA\LimpaIA.exe'
	$shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'LimpaIA.lnk'

	if ($env:LIMPAIA_REMOVER) {
		Remove-Item Env:LIMPAIA_REMOVER
		if (Test-Path $installed) {
			# roda de uma cópia temporária: o programa não consegue apagar a si mesmo em uso
			$t = Join-Path $env:TEMP 'LimpaIA-remover.exe'
			Copy-Item $installed $t -Force
			& $t --desinstalar --silencioso | Out-Null
			Remove-Item $t -ErrorAction SilentlyContinue
		}
		Remove-Item $shortcut -ErrorAction SilentlyContinue
		Write-Host "LimpaIA removido (programa, atalho e item 'Remover metadados de IA' do botão direito)."
		return
	}

	$tmp = Join-Path $env:TEMP ('LimpaIA-' + [guid]::NewGuid().ToString('N') + '.exe')
	try {
		Write-Host "Baixando LimpaIA $tag..."
		Invoke-WebRequest -UseBasicParsing -Uri "$base/LimpaIA.exe" -OutFile $tmp
		if ((Get-FileHash $tmp -Algorithm SHA256).Hash -ne $sha256) {
			throw "o arquivo baixado não confere (SHA-256 diferente). Nada foi instalado; tente de novo."
		}
		Unblock-File $tmp

		Write-Host "Adicionando 'Remover metadados de IA' ao botão direito do Explorer..."
		# --instalar copia o programa para %LOCALAPPDATA%\LimpaIA e grava o menu (HKCU)
		& $tmp --instalar --silencioso | Out-Null
		if ($LASTEXITCODE -or -not (Test-Path $installed)) { throw "não foi possível instalar o programa." }

		$sh = (New-Object -ComObject WScript.Shell).CreateShortcut($shortcut)
		$sh.TargetPath = $installed
		$sh.WorkingDirectory = Split-Path $installed
		$sh.Description = 'Remove o selo de IA (C2PA) de fotos JPEG, PNG e WebP'
		$ico = Join-Path (Split-Path $installed) 'LimpaIA.ico'
		if (Test-Path $ico) { $sh.IconLocation = $ico }
		$sh.Save()
	} catch {
		Write-Host "`nErro: $($_.Exception.Message)" -ForegroundColor Red
		return
	} finally {
		Remove-Item $tmp -ErrorAction SilentlyContinue
	}

	Write-Host @"

Pronto! LimpaIA $tag instalado.

Como usar:
  - Botão direito numa pasta, ou em fotos selecionadas (JPG, PNG, WebP) > "Remover metadados de IA"
    (no Windows 11: botão direito > "Mostrar mais opções");
  - ou abra o LimpaIA pelo Menu Iniciar e escolha a pasta.
Só as fotos que tinham o selo de IA são gravadas, na subpasta SEM_METADATA_IA; os originais não mudam.
Uma vez por semana o LimpaIA consulta o GitHub para avisar se há versão nova (nada sobre as fotos é enviado).

Uso consciente: o LimpaIA protege fotos autorais, captadas por câmera, que
receberam o rótulo "Feito com IA" por causa de ajustes finos de edição. Não use
para esconder que uma imagem foi gerada por IA, enganar o público ou remover a
marcação de imagens de outras pessoas. Quem publica é responsável pelo uso.

LimpaIA, por Alberth Palhares - https://github.com/{{REPO}}

Para remover:
  `$env:LIMPAIA_REMOVER=1; irm https://github.com/{{REPO}}/releases/latest/download/install.ps1 | iex
"@
}
