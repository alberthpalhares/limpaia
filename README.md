# LimpaIA

Remove dos JPEGs o selo de "feito com IA" (C2PA / Content Credentials) **sem recomprimir a foto**.
Autoria, copyright, Exif e perfil de cor são mantidos; os originais não são alterados. As cópias limpas
vão para a subpasta `JPEG_SEM_METADATA_IA`, junto de um relatório.

## Instalar com um comando

**Mac**: abra o app Terminal, cole e tecle Enter:

```sh
curl -fsSL https://github.com/alberthpalhares/limpaia/releases/latest/download/install.sh | sh
```

**Windows**: abra o PowerShell, cole e tecle Enter:

```powershell
irm https://github.com/alberthpalhares/limpaia/releases/latest/download/install.ps1 | iex
```

Os instaladores baixam a versão mais recente, conferem o SHA-256, instalam só para o seu usuário (sem
administrador) e adicionam "Remover metadados de IA" ao botão direito. Assim o macOS e o Windows não
mostram o aviso de programa sem assinatura. Para remover, veja o fim da mensagem do instalador ou os `LEIA-ME`.

**Sem instalar nada**: baixe [`LimpaIA.html`](https://github.com/alberthpalhares/limpaia/releases/latest/download/LimpaIA.html)
e abra no navegador. Funciona offline, e as fotos não saem do computador.

Todos os arquivos estão em [Releases](https://github.com/alberthpalhares/limpaia/releases/latest).
Documentação técnica e como gerar os pacotes: [LEIAME.md](LEIAME.md).

Remover o selo do arquivo não garante que uma rede social não detecte IA por outros meios.

Licença: [MIT](LICENSE).
