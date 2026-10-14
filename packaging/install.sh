#!/bin/sh
# Instalador do LimpaIA para macOS. Uso (no Terminal):
#   curl -fsSL https://github.com/{{REPO}}/releases/latest/download/install.sh | sh
#   curl -fsSL https://github.com/{{REPO}}/releases/latest/download/install.sh | sh -s -- --remover
#
# Baixado pelo curl, o programa não recebe a marca de quarentena, então o macOS
# não mostra o aviso do Gatekeeper. O arquivo é conferido pelo SHA-256 antes de
# instalar. Instala em /Applications ("Aplicativos"), ou em ~/Applications se o
# usuário não for administrador, sem pedir senha.
# Gerado por tools/mkinstall a partir de packaging/install.sh.
set -eu

TAG='{{TAG}}'
SHA256='{{SHA256_MAC}}'
URL="${LIMPAIA_BASE_URL:-https://github.com/{{REPO}}/releases/download/$TAG}/LimpaIA-macOS.zip"
APPS=/Applications
[ -w "$APPS" ] || APPS="$HOME/Applications"
APP="$APPS/LimpaIA.app"
BIN="$APP/Contents/MacOS/LimpaIA"

fail() { printf '\nErro: %s\n' "$1" >&2; exit 1; }

[ "$(uname -s)" = Darwin ] || fail "este instalador é para macOS. No Windows, use o comando do PowerShell."

if [ "${1:-}" = --remover ]; then
	for a in /Applications/LimpaIA.app "$HOME/Applications/LimpaIA.app"; do
		[ -x "$a/Contents/MacOS/LimpaIA" ] && "$a/Contents/MacOS/LimpaIA" --desinstalar --silencioso >/dev/null 2>&1 || true
		[ -d "$a" ] && rm -rf "$a" 2>/dev/null || true
	done
	printf 'LimpaIA removido (app e item "Remover metadados de IA" do botão direito).\n'
	exit 0
fi

ver=$(sw_vers -productVersion)
[ "${ver%%.*}" -ge 12 ] || fail "o LimpaIA precisa do macOS 12 ou mais novo (este Mac tem $ver)."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Baixando LimpaIA %s...\n' "$TAG"
curl -fsSL --retry 2 -o "$tmp/LimpaIA-macOS.zip" "$URL" || fail "não foi possível baixar $URL"

got=$(shasum -a 256 "$tmp/LimpaIA-macOS.zip" | cut -d ' ' -f 1)
[ "$got" = "$SHA256" ] || fail "o arquivo baixado não confere (SHA-256 diferente). Nada foi instalado; tente de novo."

ditto -x -k "$tmp/LimpaIA-macOS.zip" "$tmp/x"
[ -x "$tmp/x/LimpaIA.app/Contents/MacOS/LimpaIA" ] || fail "pacote incompleto."
xattr -dr com.apple.quarantine "$tmp/x/LimpaIA.app" 2>/dev/null || true
"$tmp/x/LimpaIA.app/Contents/MacOS/LimpaIA" --help >/dev/null 2>&1 || fail "o programa não roda neste Mac."

mkdir -p "$APPS"
rm -rf "$APP"
mv "$tmp/x/LimpaIA.app" "$APP"

printf 'Adicionando "Remover metadados de IA" ao botão direito do Finder...\n'
"$BIN" --instalar --silencioso >/dev/null 2>&1 || printf '(não foi possível criar o menu; o app funciona mesmo assim)\n'

cat <<EOF

Pronto! LimpaIA $TAG instalado em $APP

Como usar:
  - Botão direito numa pasta com fotos > Ações Rápidas > "Remover metadados de IA"
    (se não aparecer: Ajustes do Sistema > Teclado > Atalhos de Teclado >
    Serviços > Arquivos e Pastas > marque "Remover metadados de IA");
  - ou abra o LimpaIA (Spotlight: Cmd+Espaço e digite LimpaIA) e escolha a pasta.
As fotos limpas ficam na subpasta JPEG_SEM_METADATA_IA; os originais não mudam.

Para remover:
  curl -fsSL https://github.com/{{REPO}}/releases/latest/download/install.sh | sh -s -- --remover
EOF
