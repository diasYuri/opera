# Contribuições visuais de extensões

O chat aceita contribuições declarativas no bloco de um módulo WASM em `extensions.toml`. O formato reutiliza o contrato de módulo que já controla ativação e consentimento. A contribuição precisa vir de um módulo ativo e consentido; no modo offline, contribuições não são carregadas.

```toml
[[wasm]]
id = "example"
directory = "/absolute/path/to/extensions/example"
grants = ["logs"]
enabled = true
consent = "<digest aprovado pelo Opera>"

[[wasm.ui]]
id = "header-note"
kind = "header"
text = "Build tools"

[[wasm.ui]]
id = "review-widget"
kind = "widget"
text = "Review mode enabled"

[[wasm.ui]]
id = "skill-renderer"
kind = "renderer"
target = "skill"
text = "Skill"

[[wasm.ui]]
id = "command-completion"
kind = "autocomplete"
trigger = ":review"
value = ":review "
text = ":review"
detail = "Review checklist"
```

## Tipos

- `header`, `footer` e `widget` exibem uma linha de texto.
- `renderer` substitui o label de uma superfície conhecida. Targets: `assistant`, `user`, `thinking`, `tool`, `error`, `system`, `skill`, `agent`, `compaction`, `branch` e `announcement`. O Opera continua controlando parsing, corpo, estado, cor e wrapping.
- `autocomplete` adiciona uma opção ao completion quando o token atual começa com `trigger`. O valor apenas entra no editor após confirmação da pessoa; nunca é executado automaticamente.

## Limites e lifecycle

- Até 24 declarações por módulo e 32 contribuições ativas no total.
- Cada campo textual aceita até 256 bytes, exceto trigger (32) e valor de autocomplete (128); linhas, ESC e outros controles são rejeitados.
- A TUI limita header a 2 linhas, footer a 1 linha, widgets a 3 linhas e opções de autocomplete ao limite geral de 6.
- As contribuições são lidas uma vez quando a tela de chat inicia e permanecem como snapshot até a tela encerrar. `NO_COLOR`, largura/altura atuais e clipping são aplicados pelo host.
- O renderer nunca imprime texto de extensão como sequência de terminal. Tipos/targets não reconhecidos são ignorados.

Essa primeira versão não permite código de renderer ou callbacks executados dentro da TUI. Isso mantém o contrato atual de WASM e dá extensibilidade visual sem criar uma nova capacidade de terminal ou execução.
