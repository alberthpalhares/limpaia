# LimpaIA: documentação técnica

Guia para quem vai compilar, alterar ou publicar o LimpaIA. Para usar, veja o [README](README.md).
Criado por Alberth Palhares · Apache-2.0 (veja `LICENSE` e `NOTICE`).

## O que é removido
Remove **somente** os indicadores de IA, em JPEG, PNG e WebP, no nível de segmentos/chunks (sem decodificar nem
recomprimir a imagem):

| Formato | C2PA / Content Credentials | XMP |
|---|---|---|
| JPEG (`internal/jpegclean`) | segmentos APP11/JUMBF do manifest; corrige o APP2/MPF | APP1 padrão e estendido (inclusive órfão) |
| PNG (`internal/pngclean`) | chunks `caBX` | `iTXt` `XML:com.adobe.xmp` (com ou sem zlib), CRC recalculado |
| WebP (`internal/webpclean`) | chunk `C2PA` | chunk `XMP `, tamanho RIFF e padding corrigidos |

No XMP (`internal/xmpai`): `DigitalSourceType` de IA, `crs:fill_method="firefly"` (Lightroom) e `dcterms:provenance`
apontando para C2PA. Autoria, copyright, Exif, IPTC, ICC e orientação ficam intactos. Cada resultado é validado
(dimensões iguais, todo o resto idêntico byte a byte, reexame de marcadores) em `internal/runner`.

**Só as fotos com indicação de IA são gravadas** na subpasta `SEM_METADATA_IA`; as demais aparecem no relatório
(`SEM_IA`) e não são copiadas. Sem nenhuma foto com IA, nada é criado.

## Estrutura
- `cmd/limpaia`: o app (Windows/macOS). `selection.go` junta as fotos selecionadas no Explorer (um processo por
  foto) num lote só; `update.go` faz a verificação semanal de versão e a atualização.
- `cmd/limpaia-web`: o mesmo núcleo compilado para WebAssembly (versão no navegador).
- `web/index.html`: modelo da página web. `tools/mkweb` embute o WASM, o ícone e a versão em `dist/LimpaIA.html`.
- `tools/mkmac`: binário universal (arm64 + x86_64) e `LimpaIA.app` com ícone. `tools/mkicon`: desenha o ícone
  (`assets/icon/`, com variantes `-variante a..e` para comparar). `tools/mkinstall`: instaladores de uma linha.
- `packaging/`: LEIA-ME de cada sistema e modelos `install.sh` / `install.ps1`.
- `docs/img/`: imagens do README.

## Compilar e publicar
- `.\build.ps1 [-Versao X.Y.Z]`: vet (Windows, macOS, Linux, js/wasm), testes, ícone e versão no `.exe`
  (`go-winres`, baixado pelo `go run` na primeira vez), `dist\LimpaIA.exe`, `dist\LimpaIA-macOS.zip`, `dist\LimpaIA.html`.
- `.\release.ps1 -Versao X.Y.Z [-Novidades '...']`: exige a árvore git limpa; roda o build, gera
  `install.sh`/`install.ps1`/`SHA256SUMS.txt` com a versão e os SHA-256 embutidos, cria a tag e a release.
  A release dispara `.github/workflows/pages.yml`, que publica o `LimpaIA.html` em
  https://alberthpalhares.github.io/limpaia/.
- `.github/workflows/ci.yml`: testes em Linux, Windows e macOS, e instalação real com o `install.sh` num Mac
  (confere o `.app`, o `.icns`, o binário universal e os plists da Ação Rápida).

## Instaladores de uma linha
- **Mac:** `curl -fsSL https://github.com/alberthpalhares/limpaia/releases/latest/download/install.sh | sh`
- **Windows:** `irm https://github.com/alberthpalhares/limpaia/releases/latest/download/install.ps1 | iex`

Arquivos baixados por `curl`/`Invoke-WebRequest` não recebem a marca de quarentena (Mac) nem o Mark of the Web
(Windows), que é o que dispara os avisos do Gatekeeper/SmartScreen. Cada instalador aborta se o SHA-256 não
conferir. O `.ps1` gerado é só ASCII (o GitHub serve sem charset e o PowerShell 5.1 estragaria os acentos).
Remoção: `… | sh -s -- --remover` (Mac) e `$env:LIMPAIA_REMOVER=1; irm … | iex` (Windows).
Testes sem publicar: `LIMPAIA_BASE_URL` troca a URL dos pacotes.

## Menu de contexto
- **Windows** (HKCU, sem admin): verbos em `Directory`, `Directory\Background` e
  `SystemFileAssociations\.{jpg,jpeg,png,webp}` (`MultiSelectModel=Document`, `--selecao`). O ícone do menu é o
  `LimpaIA.ico` gravado ao lado da cópia em `%LOCALAPPDATA%\LimpaIA`. No Windows 11 fica em "Mostrar mais opções"
  (o menu novo exige app empacotado e assinado). Janelas e registro usam a API nativa (sem PowerShell oculto nem `reg.exe`).
- **macOS:** Ação Rápida "Remover metadados de IA" (pastas, JPEG, PNG, WebP), chamando a cópia em
  `~/Library/Application Support/LimpaIA` uma vez com todos os itens. O Finder mostra o ícone padrão das Ações
  Rápidas; o ícone do LimpaIA aparece no app. **O app do Mac não foi testado num Mac real**, só no CI do GitHub.

## Atualização automática
No máximo uma vez por semana (`ultima_verificacao` em `UserConfigDir/LimpaIA`), ao abrir o app ou usar o menu,
**depois** de processar as fotos, consulta `api.github.com/repos/alberthpalhares/limpaia/releases/latest`.
Se houver versão nova, pergunta (Atualizar / Depois / Não avisar mais), baixa o pacote e o `SHA256SUMS.txt` da
release, confere o hash e troca o programa (Windows: renomeia o `.exe` em uso para `.old`; Mac: troca o `.app`).
Desliga com "Não avisar mais" (arquivo `nao_atualizar`) ou `LIMPAIA_SEM_ATUALIZACAO=1`.
Testes: `LIMPAIA_URL_ATUALIZACAO` e `LIMPAIA_ATUALIZAR_SEM_PERGUNTAR=1`.

## Linha de comando
`LimpaIA <pasta|fotos>...`, `--verificar` (só diagnostica), `--instalar`, `--desinstalar`, `--versao`.
PowerShell: `.\Remover-MetadataIA.ps1 "D:\Fotos\Evento"`.

## Avisos do sistema (sem assinatura paga)
Baixados pelo navegador, o `.exe` e o `.zip` mostram um aviso do SmartScreen/Gatekeeper na primeira abertura
(os LEIA-ME explicam como liberar). Só some de vez com assinatura: Apple Developer ID + notarização (US$ 99/ano)
e certificado de assinatura de código no Windows. O instalador de uma linha e a versão web não têm esse aviso.

## Limites
Remove o que está no arquivo; as plataformas ainda podem detectar IA por outros meios. Limite de 1 GiB por foto
no app e de 256 MB na versão web.

## Uso consciente
O LimpaIA protege fotos autorais, captadas por câmera, que receberam o rótulo "Feito com IA" por causa de
ajustes finos de edição. Não deve ser usado para esconder que uma imagem foi gerada por IA, enganar o público ou
remover a marcação de imagens de outras pessoas. Texto completo no README e no `NOTICE`.
