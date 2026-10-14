# LimpaIA — remove o selo "feito com IA" dos seus JPEGs

Remove **somente** o manifest C2PA / Content Credentials (e marcas de IA no XMP: `DigitalSourceType` de IA e
`crs:fill_method="firefly"` do Lightroom). Não recomprime: os dados da imagem são copiados byte a byte.
Preserva autoria, copyright, Exif, IPTC, perfil ICC e orientação. Originais nunca são alterados;
as cópias vão para a subpasta `JPEG_SEM_METADATA_IA` com os mesmos nomes, junto de um relatório (.txt/.csv).

## Instalar com um comando (recomendado: sem aviso do Gatekeeper/SmartScreen)
- **Mac:** `curl -fsSL https://github.com/alberthpalhares/limpaia/releases/latest/download/install.sh | sh`
- **Windows:** `irm https://github.com/alberthpalhares/limpaia/releases/latest/download/install.ps1 | iex`

Arquivos baixados por `curl`/`Invoke-WebRequest` não recebem a marca de quarentena (Mac) nem o Mark of the Web
(Windows), que é o que dispara os avisos. Cada instalador traz a versão e o SHA-256 do pacote embutidos
(`tools\mkinstall`) e aborta se o download não conferir. Modelos: `packaging\install.sh` e `packaging\install.ps1`
(o `.ps1` gerado é só ASCII: o GitHub serve sem charset e o PowerShell 5.1 estragaria os acentos).
Remoção: `… | sh -s -- --remover` (Mac) e `$env:LIMPAIA_REMOVER=1; irm … | iex` (Windows).
Para testar sem publicar, `LIMPAIA_BASE_URL` troca a URL dos pacotes.

**Publicar versão:** `.\release.ps1 -Versao 1.0.1` (exige a árvore git limpa; roda o build, gera os instaladores,
cria a tag e a release com todos os arquivos de `dist\`).

## O que enviar a quem vai usar (pasta `dist\`)
- **Windows:** `LimpaIA.exe` + `LEIA-ME_Windows.txt`.
- **macOS:** `LimpaIA-macOS.zip` (já contém `LimpaIA.app` universal para chip Apple e Intel, e o `LEIA-ME.txt`).
- **Web (qualquer sistema, recomendado no Mac):** `LimpaIA.html` + `LEIA-ME_Web.txt`. Sem aviso do Gatekeeper/SmartScreen.

## Web — `LimpaIA.html`
- Uma página autocontida (~1,6 MB): o mesmo código Go compilado para WebAssembly, rodando offline no navegador.
  As fotos não saem do computador. Dá para enviar por e-mail/WhatsApp ou hospedar em qualquer lugar.
- **Chrome/Edge:** grava `JPEG_SEM_METADATA_IA` + relatório dentro da pasta escolhida (File System Access API).
  **Safari 16.4+/Firefox:** baixa `JPEG_SEM_METADATA_IA.zip` com o mesmo conteúdo.
- Sem menu do botão direito; limite de 256 MB por foto. `web\index.html` é o modelo; `tools\mkweb` embute o WASM.

## Windows
- **Duplo clique** em `LimpaIA.exe` → pergunta se quer o menu do botão direito → escolha a pasta.
- `--instalar` / `--desinstalar` adicionam/removem "Remover metadados de IA" (sem admin, só HKCU). O programa se copia
  para `%LOCALAPPDATA%\LimpaIA`, então o menu continua valendo se o `.exe` baixado for apagado.
- Ou arraste a pasta sobre o `LimpaIA.exe`. PowerShell: `.\Remover-MetadataIA.ps1 "D:\Fotos\Evento"`.
- Sem PowerShell oculto nem `reg.exe`: janelas e registro usam a API nativa do Windows (menos falsos positivos de antivírus).

## macOS — NÃO testado em Mac real
- `LimpaIA.app` (binário universal, macOS 12+). Duplo clique → pergunta pelo menu → seletor de pasta.
  `--instalar` cria a Ação Rápida "Remover metadados de IA" (Finder → botão direito → Ações Rápidas);
  o programa se copia para `~/Library/Application Support/LimpaIA`.

## Avisos do sistema (sem assinatura paga)
Sem certificado, o Windows SmartScreen e o Gatekeeper do macOS mostram um aviso **na primeira abertura** de um
arquivo baixado; os `LEIA-ME` explicam como liberar (1 vez). Só some de vez com assinatura: Apple Developer ID +
notarização (US$ 99/ano) no Mac e certificado de assinatura de código (p.ex. Azure Trusted Signing) no Windows.

## Limites
Remove o que está no arquivo; plataformas ainda podem detectar IA por outros meios. `--verificar` só diagnostica.
Código: `.\build.ps1` (vet + testes + exe + zip do Mac + página web). `tools\mkmac` monta o binário universal e o `.app`.
