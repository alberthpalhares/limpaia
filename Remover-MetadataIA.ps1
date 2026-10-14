# Copyright 2026 Alberth Palhares - SPDX-License-Identifier: Apache-2.0
<#
.SYNOPSIS  Remove o indicador de IA (C2PA / Content Credentials) de fotos JPEG, PNG e WebP sem recomprimir.
.EXAMPLE   .\Remover-MetadataIA.ps1 "D:\Fotos\Eventos\Evento XYZ"
           .\Remover-MetadataIA.ps1 "D:\Fotos\Evento" -Verificar   # só diagnostica
Saída: subpasta SEM_METADATA_IA (só as fotos com indicação de IA); originais ficam intactos.
#>
param([Parameter(Mandatory, Position = 0)][string]$Pasta, [switch]$Verificar)
$exe = Join-Path $PSScriptRoot 'LimpaIA.exe'
if (-not (Test-Path $exe)) { $exe = Join-Path $PSScriptRoot 'dist\LimpaIA.exe' }
if (-not (Test-Path $exe)) { throw 'LimpaIA.exe não encontrado ao lado deste script.' }
$a = @('--silencioso', $Pasta); if ($Verificar) { $a = @('--verificar') + $a }
& $exe @a
exit $LASTEXITCODE
