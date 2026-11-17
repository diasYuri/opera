# Plano de paridade de UI: Pi → Opera

**Status:** implementação P0, P1 e P2 concluída; suítes automatizadas, check, lint, build, vet e smokes PTY aprovados. Os payloads Kitty/iTerm2 têm teste automatizado; falta inspeção visual em emulador compatível. A tentativa de abrir iTerm2 pelo controle de UI foi bloqueada pela política de segurança do ambiente.
**Escopo da referência:** modo interativo de terminal do `packages/coding-agent` em `/Users/yuri/git/other/pi-mono`.  
**Escopo do produto:** chat TUI do Opera em `internal/ui` e superfícies interativas que o CLI expõe. Esta avaliação não trata os apps web/macOS `p2` como UI do agente Pi.

## Objetivo

Desenhar e implementar, em etapas, as capacidades de interface do Pi que melhoram a experiência equivalente no Opera: apresentação de conversas, inspeção de ferramentas, navegação de sessões, seleção/configuração, feedback de atividade, extensibilidade visual, acessibilidade e estados de erro/carregamento. A paridade é funcional e conceitual; copiar aparência, marcas ou cada Easter egg do Pi não é requisito.

## Resumo da avaliação

Opera já tem uma TUI real com editor multilinha, streaming, status/spinner, indicação de atividade, cabeçalho de workspace/modelo, autocomplete simples de comandos, seletor de modelos, login OAuth com estados de progresso/cancelamento, painéis de texto roláveis, aprovação/negação, retry, steering/follow-up, atalhos contextuais, redimensionamento e modo sem movimento. A UI também mantém texto transmitido limitado no viewport e usa scrollback para mensagens concluídas.

Pi apresenta uma camada de interação mais rica e modular: conversa persistente composta por componentes, Markdown progressivo, blocos de thinking e chamadas de ferramenta, detalhes recolhíveis, renderização de diff e resultados, navegação de árvore/branch de sessão, busca e retomada de sessão, seletor de mensagens do usuário, autocomplete contextual de arquivos/comandos, login/seletores ricos, settings editáveis, temas, confiança por projeto, seletor de imagens, indicadores/status extensíveis, renderer substituível e componentes fornecidos por extensions. A diferença central é que Opera hoje é uma conversa de sessão curta renderizada como tail + scrollback terminal, enquanto Pi mantém um viewport de conversa com componentes que podem ser atualizados, navegados e reconfigurados.

## Inventário comparativo

Legenda: **Existe** = equivalente funcional presente; **Parcial** = há versão menor ou sem a mesma affordance; **Gap** = não localizado na UI atual do Opera; **Decidir** = valor de produto incerto.

| Área | Pi | Opera hoje | Estado / decisão |
|---|---|---|---|
| Conversa e histórico visual | `chat-viewport.ts`, `interactive-mode.ts`, componentes por mensagem; mensagens são componentes persistentes | `internal/ui/layout.go`: apenas tail limitado durante stream; mensagens finais vão para scrollback via `tea.Println` em `internal/ui/chat.go` | **Gap:** viewport próprio, scroll explícito, histórico navegável e âncoras de conversa. Prioridade P0. |
| Mensagens do usuário/assistant | Componentes próprios, Markdown e formatação de cada papel; `assistant-message.ts`, `user-message.ts` | Blocos plain text em `messageBlock`, sem renderer Markdown | **Gap:** Markdown, code fences, tabelas/listas e destaque de sintaxe; manter fallback legível sem cor. P0. |
| Thinking / reasoning visível | Slots de thinking e conteúdo assistant em `assistant-message.ts`; indicação de estado | Status da execução genérico; reasoning não tem bloco próprio | **Parcial/Decidir:** se Opera já recebe reasoning autorizado, expor como seção recolhível e claramente rotulada; não fabricá-lo. P1. |
| Ferramentas e shell | `tool-execution.ts`, `bash-execution.ts`: estado, argumentos resumidos, saída progressiva e apresentação específica por ferramenta | UI mostra status `Tool: <nome>` e resultados resumidos/texto; aprovação tem painel | **Parcial:** cards/blocos por operação, output incremental, recolher/expandir, resultado/falha/duração e identificação consistente. P0. |
| Diff | `diff.ts` com renderização de alterações para operações | Aprovação/detalhes usam painéis textuais genéricos; há backend de git/diff, sem componente de diff dedicado confirmado | **Gap:** diff legível com linhas adicionadas/removidas, contexto e limites de largura. P0 para aprovação. |
| Navegação da sessão | `tree-selector.ts`: árvore de mensagens/branches e preview; `/tree` | Sessões e branches são recursos CLI/backend, UI atual não apresenta árvore | **Gap:** navegar branches/checkpoints e retornar a ponto selecionado, preservando confirmação antes de operação destrutiva. P1. |
| Retomar sessões / localizar mensagens | `session-selector.ts`, `session-selector-search.ts`, `user-message-selector.ts` | `sessions list/inspect/export/branch` é CLI; chat aceita request de retomada, mas não oferece picker visual de histórico | **Gap:** picker com busca, metadados, preview e retomada; busca/navegação de mensagens. P1. |
| Seleção e autocomplete | Seletores específicos e editor com autocomplete providers; key hints configuráveis | Seletor de modelo e completions de slash commands; sem completion de paths/mentions confirmada | **Parcial:** unificar navegação, busca/filtro, preview e hints; adicionar completion de arquivos/workspace se suporte de runtime permitir. P1. |
| Modelo/provedor/login | `model-selector.ts`, `scoped-models-selector.ts`, `oauth-selector.ts`, `login-dialog.ts`, `auth-url.ts`, `first-time-setup.ts`, brand e estados de autenticação | Login OAuth e picker de providers/models; estados essenciais tratados em `internal/ui/login.go` | **Parcial:** identidade/descrições/estado de sessão, login com instruções navegáveis, status de conexão e transições de volta previsíveis. P1. |
| Settings, thinking e confiança | `settings-selector.ts`, `settings-submenu.ts`, `thinking-selector.ts`, `trust-selector.ts`, `config-selector.ts` | `/settings`, `/thinking`, `/context` abrem saídas textuais; configurações persistem fora de UI rica | **Gap:** controles editáveis com valor atual, escopo, explicação e confirmação apropriada. Confiança/política exige UX explícita e não deve parecer decorativa. P1. |
| Tema e aparência | `theme/` (tokens, dark/light/system, schema/controller), `theme-selector.ts`, componentes themed | `internal/ui/theme.go`: paleta escura/clara e `NO_COLOR`, sem seletor ou tokens persistidos visíveis | **Parcial:** tokens semânticos, tema selecionável/persistido, cor do terminal respeitada e contraste verificado. P2. |
| Estados de espera/progresso | `status-indicator.ts`, `bordered-loader.ts`, `countdown-timer.ts`; loaders com contexto/atalhos | Spinner, status, progresso de atividade indeterminado e texto de login; `--no-motion` | **Parcial:** estados distintos para inicialização, espera de provider, ferramenta, approval, login e retry; duração/transição e cancelamento sempre claros. Nunca exibir percentual falso. P0. |
| Animação e feedback | loaders/spinners, atualização incremental do assistant e indicadores; animações são localizadas em componentes | spinner + traço de atividade com Harmonica, geração de timer invalidada por estado, `--no-motion` | **Existe em base; parcial em cobertura:** consolidar semântica dos estados, transições sem flicker e reduzir motion respeitando flag. P0. |
| Barra inferior e atalhos | `footer.ts`, `keybinding-hints.ts`, status; hints podem refletir bindings efetivos | `helpLine()` muda por largura/estado; instruções de painel/editor | **Parcial:** rodapé responsivo com contexto atual, bindings configurados e prioridades, sem overflow em terminal estreito. P1. |
| Erros, cancelamento e recovery | UI por entrada/operação, mensagens específicas e fluxo para retry/session | cancelamento, erro, approval e retry já existem; detalhes via painel | **Parcial:** erro inline acionável, código/referência de operação, retry elegível, estados incertos/reconciliação claramente distintos. P0. |
| Imagens e conteúdo visual | `show-images-selector.ts`, `image.ts` do TUI compartilhado, imagem inline quando terminal suporta | backend aceita anexos multimodais; renderer visual específico não localizado | **Gap/Decidir:** preview quando terminal/capacidade suportar; fallback com tipo, dimensões e indicação de conteúdo anexado. P2. |
| Extensões de UI | API em `interactive-mode.ts`: custom renderer/editor, header/footer, shortcuts, autocomplete, widgets e selectors; vários componentes em `components/` | extensões e comandos/backend existem, mas não foi localizada API equivalente para injetar componentes no chat TUI | **Gap:** definir pontos de extensão visuais limitados e lifecycle seguro; só depois das superfícies nativas estáveis. P2. |
| Conteúdo avançado | `markdown-transform.ts`, `mermaid.ts`, skill/compaction/branch summary, announcements, custom messages | texto e painéis; comandos de compact/tasks/agents disponíveis, sem componentes de resumo dedicados identificados | **Parcial:** mensagens de sistema tipadas (compactação, branch, skill, agente, aviso), com visual discreto e leitura no histórico. Diagramas somente se renderer terminal suportar e fallback existir. P2. |
| Acessibilidade e terminal | renderer TUI compartilhado e keybindings configuráveis; comportamento visual adaptado a terminal | `--accessible`, `NO_COLOR`, `--no-motion`, clipping/Unicode, teclado e resize | **Base existe; gap de equivalência:** verificar todas as novas telas com teclado, sem cor, motion reduzido, Unicode e largura/altura mínima. Não remover essas garantias. P0 transversal. |

### Comandos do Pi com superfície visual

O `interactive-mode.ts` confirma handlers para `/settings`, `/scoped-models`, `/model`, `/thinking`, `/export`, `/share`, `/copy`, `/hotkeys`, `/tree`, `/login`, `/new`, `/compact`, `/reload` e `/resume`. Para a comparação de UI, os mais relevantes são `/tree`, `/resume`, `/settings`, `/thinking`, `/scoped-models`, `/hotkeys` e `/login`: Pi abre seletores/diálogos próprios ou altera o estado visível da sessão. Opera já expõe partes dessas capacidades por comandos do chat/CLI e pelo seu modelo de sessão durável, mas não um catálogo visual integrado equivalente. `/copy`, `/export`, `/share`, `/new` e `/reload` são ações de sessão/saída; avaliar se há gap de UX no chat, sem tratá-las automaticamente como componentes visuais ausentes.

Capacidades como extensões, ferramentas, MCP, memória, compactação, branches e exportação existem parcial ou totalmente no backend/CLI Opera. O gap avaliado é a descoberta, affordance e feedback no modo interativo; o plano não propõe duplicar essas capacidades nem implica que toda ação deva virar botão ou menu.

## Direção de design para Opera

1. **Hierarquia de leitura:** conversa é a superfície principal; header identifica workspace/modelo; composer é persistente; footer mostra apenas atalhos relevantes ao estado atual. Separar conteúdo gerado, eventos de ferramenta, alertas e metadados por estrutura e estilo, não só por cor.
2. **Uma gramática visual:** tokens semânticos `text`, `muted`, `accent`, `success`, `warning`, `error`, `border`, `added`, `removed`; suportar tema escuro/claro do terminal, `NO_COLOR` e contraste legível. Não codificar cores em cada componente.
3. **Estados explícitos:** `idle`, `starting`, `streaming`, `tool-running`, `approval-pending`, `cancelling`, `retryable-error`, `failed`, `reconciling`. Cada estado define indicador, mensagem, ações disponíveis, movimento e condição de término. Sem porcentagem para trabalho indeterminado.
4. **Motion funcional:** animação comunica atividade ou transição e nunca é necessária para compreender progresso. `--no-motion` desliga timers/animações, não só o spinner. Evitar redraw contínuo quando idle; invalidar ticks obsoletos em cancelamento/saída/troca de tela.
5. **Composição estável:** mensagens concluídas não devem duplicar entre viewport e scrollback; streaming atualiza um único bloco. Resize recalcula wrap e seleção preservando posição/âncora quando possível. Conteúdo externo passa pela sanitização já usada em `safeText`.
6. **Interações previsíveis:** toda lista tem busca/seleção/empty state/loading/error; toda tela modal tem cancelamento e hint; toda ação destrutiva ou com efeito identifica alvo e consequência. Atalhos mostrados vêm da configuração vigente.

## Plano por fases

### P0 — Fundamentos de conversa e execução

- [x] Definir modelo de apresentação de mensagem/evento em `internal/ui`: user, assistant streaming/final, reasoning autorizado, tool call/result, approval, erro, resumo e aviso. Associar ao ID/sequência existente; não duplicar estado durável.
- [x] Criar viewport de conversa com scroll próprio, navegação para início/fim, preservação da âncora durante streaming e resize; manter scrollback único e saída terminal restaurada.
- [x] Adicionar renderer Markdown incremental, wrapping por largura, code fences/destaque com fallback plain text e truncamento visual que não altere o conteúdo guardado.
- [x] Renderizar execução de ferramenta como bloco identificável com entrada resumida, estado, saída progressiva, resultado/falha e recolhimento; expor approval junto da operação correspondente.
- [x] Criar componente de diff para aprovação/revisão com `+`/`-`, contexto, arquivo, resumo e fallback para diff grande/terminal estreito. Aprovação deve continuar vinculada à operação que o backend validará.
- [x] Formalizar estados de atividade e erro: inicialização, stream, ferramenta, approval, cancelamento, falha retryable/final e in-doubt. Hints e ações devem acompanhar cada estado.
- [x] Consolidar animação/spinner: um ciclo por trabalho ativo, cancelável, sem ticks em idle e compatível com `--no-motion`; `NO_COLOR` afeta todo elemento novo.
- [x] Fazer smoke manual em PTY para stream, ferramenta/progresso/resultado, approval, erro, Unicode, fila, `NO_COLOR`, `--no-motion`, restauração ao sair, tamanhos 80×24/40×12/20×8 e resize ao vivo entre essas dimensões.
- [x] Validar cancelamento efetivo com executor cancelável e executar a suíte de UI. O smoke PTY confirmou a transição `Running` → `Cancelling…` → `Cancelled`; `TestCtrlCCancelsExecutionAndWaitsForCancelledResult` verifica que a UI espera o resultado cancelado antes de liberar retry.

### P1 — Navegação, retomada e configurações

- [x] Seletor de sessões com busca incremental, workspace, título/última atividade, preview e estados vazio/carregando/erro; retomar sem replay de efeitos.
- [x] Navegador de árvore/histórico para branches, checkpoints, mensagens e retomada, integrado às identidades persistentes do Opera; indicar claramente o destino de cada ação.
- [x] Seletor de mensagens do usuário e navegação entre ocorrências; evitar duplicar uma busca que já possa ser atendida pelo viewport.
- [x] Evoluir model/provider picker: busca, seleção atual, compatibilidade/status e instruções de login contextualizadas; preservar o fluxo de cancelamento existente.
- [x] Promover `/settings`, `/thinking`, `/context` e política para controles visuais com valor, origem/escopo, descrição e atualização persistida; confirmação para mudanças que afetam segurança/execução.
- [x] Completar autocomplete de comandos com descrição e hints; avaliar completion de arquivos e menções apenas após confirmar API de filesystem/workspace e custo em diretórios grandes.
- [x] Unificar footer e hints responsivos por modo e largura; obter rótulos de bindings configurados em vez de hardcode novo.

Implementação P1 em `internal/ui` e `internal/cli/chat_session.go`; detalhes e verificação estão em [p1-implementation.md](p1-implementation.md). O smoke PTY abriu `/settings`, `/resume` e `/tree`; testes da camada UI/serviço verificam seleção por teclado e retomada, criação do branch a partir do checkpoint e persistência/retorno de configuração.

### P2 — Identidade visual e extensibilidade

- [x] Extrair tokens/componentes visuais e suportar seletor/persistência de tema escuro/claro/sistema conforme disponibilidade do terminal; conservar `NO_COLOR`.
- [x] Adicionar preview de anexos/imagens com detecção de capacidade do terminal e fallback textual consistente; o conteúdo não pode depender de imagens para ser compreendido.
- [x] Tipar eventos visuais para compaction, branch, skills, agentes/subagentes e avisos de sistema, diferenciando-os da fala normal do assistant.
- [x] Definir uma API declarativa para extensões visuais (header/footer/widgets/renderers e autocomplete) com lifecycle, limites de largura/altura e sanitização; reusar o contrato de WASM já existente.
- [x] Avaliar diagramas Mermaid/Easter eggs após os fluxos essenciais. Foram adiados como opcionais: o renderer terminal atual não fornece Mermaid e Easter eggs não melhoram a compreensão ou a operação da TUI.

Implementação P2, limites e avaliação dos itens opcionais estão em [p2-implementation.md](p2-implementation.md). O contrato de contribuições está em [ui-extensions.md](../../../docs/ui-extensions.md).

## Critérios de aceite

- O histórico da conversa pode ser revisitado e não perde posição durante stream/resize; cada mensagem/evento aparece uma única vez.
- User e assistant se distinguem por alinhamento e tratamento visual: mensagem do usuário em fundo cinza à direita; resposta à esquerda sem rótulo de autor.
- Markdown e diffs são legíveis com cor e sem cor, em terminais largos e estreitos, sem escapes ANSI vindos de texto externo.
- A pessoa consegue distinguir o que o assistant disse, o que uma ferramenta executou, o que aguarda aprovação, o que falhou e o que pode ser retomado.
- Cada estado ativo tem status acionável e termina corretamente em sucesso, erro ou cancelamento; trabalho sem duração conhecida nunca mostra porcentagem inventada.
- Pickers e configurações têm navegação por teclado, filtros, estado vazio/carregando/erro, valor atual e retorno/cancelamento compreensível.
- Acessibilidade permanece: modo acessível, sem cor e sem movimento; nenhum redesenho periódico quando idle; UI não quebra por largura/altura, Unicode ou output longo.
- Funcionalidades de histórico, branch, uso e operação continuam lendo os registros/APIs do Opera; a UI não cria uma fonte de verdade paralela nem repete efeitos.

## Referências de código examinadas

**Pi** — `/Users/yuri/git/other/pi-mono/packages/coding-agent/src/modes/interactive/`: `interactive-mode.ts` (composição, eventos, comandos e extensões); `chat-viewport.ts` (viewport); `components/assistant-message.ts`, `user-message.ts`, `tool-execution.ts`, `bash-execution.ts`, `diff.ts`, `status-indicator.ts`, `bordered-loader.ts`, `footer.ts`, `keybinding-hints.ts`; seletores `tree-selector.ts`, `session-selector.ts`, `session-selector-search.ts`, `user-message-selector.ts`, `model-selector.ts`, `settings-selector.ts`, `thinking-selector.ts`, `theme-selector.ts`, `trust-selector.ts`, `show-images-selector.ts`; `theme/` (tokens, schema, controller e temas).

**Opera** — `internal/ui/chat.go` (eventos/ações e estado), `internal/ui/layout.go` (composição, status, animação, editor e clipping), `internal/ui/theme.go` (paleta e delegates), `internal/ui/autocomplete.go`, `internal/ui/login.go`; `internal/cli/chat_session.go`, `internal/cli/sessions.go`, `internal/cli/capabilities.go` (superfícies disponíveis no CLI que podem alimentar UI). O inventário confirma capacidades de backend que não têm ainda uma interface visual equivalente; isso não significa que devem ser reimplementadas no frontend.

**Limite da comparação:** considera a implementação em disco consultada em 2026-11-16 e o modo interativo de coding agent do Pi. Extensões externas podem adicionar telas ao Pi em runtime e não foram tratadas como conjunto finito; o plano registra a API de extensão e componentes nativos do repositório.
