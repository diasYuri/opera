# Implementação P2 — identidade visual e extensibilidade

**Status:** implementação concluída; check, lint, build, vet, suítes automatizadas e smoke manual em PTY aprovados para as superfícies exercitadas. Os protocolos Kitty/iTerm2 são verificados por testes de payload; a renderização visual no emulador em si ainda não foi inspecionada.

## Plano e resultado

| Área | Plano de implementação | Resultado |
|---|---|---|
| Tema e tokens | Adicionar `ui.theme = system|dark|light` ao schema; persistir a seleção no arquivo privado de preferências do workspace; seguir `tea.BackgroundColorMsg` apenas no modo `system`; centralizar tokens semânticos; manter `NO_COLOR` sem cor. | Implementado em `internal/config`, `internal/ui/theme.go`, e no picker `/settings`. |
| Preview de anexos | Reusar a validação de workspace e symlinks de `contextfiles.Load`; obter dimensões e metadados; habilitar Kitty/iTerm2 apenas em saída TTY identificada; exibir um preview compacto quando possível; sempre renderizar nome, tipo e tamanho como fallback. | Implementado com payload em memória limitado a 1 MiB no total e uma miniatura de até 512 KiB. O painel de anexos nunca é fonte exclusiva do conteúdo. |
| Eventos visuais | Modelar compaction, branch, skill, agente/subagente e announcement como eventos tipados separados de texto assistant; não renderizar deltas de subagente como fala do agente principal. | Implementado em `internal/ui/conversation.go` e nos fluxos de branch do chat. |
| Extensões visuais | Expor declarações `wasm[].ui` no contrato TOML existente; validar tipo, target, controles e tamanho; ler somente de módulos ativos/consentidos; normalizar outra vez no host; compor header/footer/widgets, rótulos de renderer e autocomplete com as dimensões da TUI. | Implementado como API declarativa sem callbacks de código e sem ANSI vindo de extensão. Snapshot carregado uma vez no início do chat e descartado ao encerrar a tela. |
| Mermaid/Easter eggs | Verificar se a UI atual oferece renderer de diagrama terminal confiável e se esses diferenciais resolvem um fluxo real. | Mermaid foi adiado: Glamour cobre Markdown terminal, mas não fornece renderer Mermaid; desenhar uma representação incompleta seria enganoso. Easter eggs também foram adiados por não melhorarem compreensão, acessibilidade ou operação. Código fence continua sendo um fallback textual legível. |

## Contrato visual das extensões

Consulte [ui-extensions.md](../../../docs/ui-extensions.md) para o exemplo TOML, tipos de contribuição e limites. O módulo precisa estar ativado e consentido; suas contribuições vêm das configurações locais protegidas. A TUI decide estilo, clipping, wrapping, altura e momento de atualização. O renderer declarativo permite substituir labels de superfícies já conhecidas, sem substituir o parser ou o conteúdo armazenado.

## Critérios de aceite P2

- `/settings` oferece tema `system`, `dark` e `light`; a escolha é persistida por workspace e o terminal volta a controlar o tema em `system`.
- `NO_COLOR` permanece sem cor inclusive para o editor, listas, renderer de Markdown, borda e novos eventos.
- Anexos são validados pelo mesmo caminho de workspace das requests; no Kitty/iTerm2 identificados pode aparecer uma miniatura; em qualquer outro caso há indicação textual com path/tipo/tamanho.
- Evento visual não aparece como fala do assistant; compaction, branch, skill, subagente e announcement possuem label próprio.
- Contribuições externas são texto limitado e sanitizado, não conseguem emitir ANSI, ocupar largura/altura sem limite ou instalar callbacks de terminal.
- Mermaid e efeitos decorativos não impedem a entrega das superfícies essenciais.

## Verificação feita

- `rtk go build -tags=feature_lsp,feature_wasm_ext ./...` — aprovado.
- `rtk go vet -tags=feature_lsp,feature_wasm_ext ./...` — aprovado.
- Smoke manual em PTY 80×24 confirmou tokens de tema escuro/claro, `NO_COLOR`, eventos tipados, contribuições de header/widget/footer e fallback textual do anexo. Em 40×12 e 20×8, o layout continuou limitado às dimensões do terminal.
- `TestRenderInlineImageProtocols` verifica que o renderer gera sequências Kitty Graphics e iTerm2 Inline Images válidas para um PNG e retorna vazio para protocolo não suportado. A seleção de protocolo continua restrita a uma saída TTY reconhecida.
- A renderização da miniatura dentro de uma janela Kitty/iTerm2 não foi inspecionada visualmente nesta sessão; o fallback textual e o caminho de geração do protocolo estão cobertos. Tentei abrir iTerm2 para fazer o smoke, mas o controle de UI bloqueou o bundle `com.googlecode.iterm2` por política de segurança do ambiente.
