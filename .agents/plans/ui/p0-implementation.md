# Plano de implementação — P0 da UI

Este documento quebra a fase P0 de [README.md](README.md) em fatias de implementação. É planejamento, não alteração de código. Baseado no código do Opera e na versão `github.com/diasYuri/y v1.3.4` declarada no `go.mod`.

## Estado de execução

Implementação de S1–S6 aplicada em `internal/ui`; as decisões de integração de S0 foram incorporadas: viewport inline sem alternate screen, Huh como submodelo do programa Bubble Tea e Glamour v2 (`charm.land/glamour/v2 v2.0.1`) compatível com o stack Lip Gloss v2. Follow-ups aceitos durante uma execução ficam numa pilha compacta destacada acima do composer e só entram no histórico quando o próximo run começa. Em 2026-11-16 houve smoke manual em PTYs 80×24, 40×12 e 20×8 com executor descartável: stream, ferramentas/progresso/resultado, approval, erro, Unicode, fila, `NO_COLOR`, `--no-motion`, resize ao vivo entre 80×24/40×12/20×8 e restauração ao sair. O cancelamento efetivo também foi validado no PTY com executor cancelável: a UI exibiu `Cancelling…`, aguardou o resultado cancelado e restaurou o estado retomável. A suíte automatizada passou com e sem tags e sob race detector; check, lint, build e vet passaram. A aprovação continua usando o ID/fingerprint existente do backend; a UI não executa nem reautoriza operações localmente.

## Resultado esperado

O chat mantém a conversa vista nesta execução em um viewport navegável; mensagens do assistant podem ser transmitidas e renderizadas progressivamente; chamadas e resultados de ferramentas têm blocos próprios; aprovações mostram contexto e diff legível; erro, retry, cancelamento e reconciliação têm estados distinguíveis. No histórico, a mensagem do usuário usa fundo cinza e alinhamento à direita; a resposta do Opera fica à esquerda sem rótulo de autor. O estado durável continua pertencendo a `internal/app`/Y; a UI projeta eventos e não inicia efeitos por conta própria.

## Estado atual confirmado

- `internal/ui/chat.go` centraliza o estado da apresentação no modelo Bubble Tea, processa `app.Update` e dispara I/O através de `tea.Cmd`. Desduplica evento por `Sequence`, acumula texto de stream em `m.text` e mantém mensagens/eventos no viewport; `tea.Println` fica reservado ao banner e a saídas que não pertencem à conversa.
- `internal/ui/layout.go` define `tea.View` com `AltScreen = false`; a vista dinâmica mostra apenas até 8 linhas do tail durante stream. Já usa `bubbles/viewport` para painéis, `bubbles/spinner`, `lipgloss`, `ansi.Hardwrap`, `safeText`, `harmonica` e timers condicionados ao estado. Resize recalcula dimensões; `--no-motion` e `NO_COLOR` já são tratados.
- `internal/ui/theme.go` centraliza tokens semânticos para temas claro/escuro, incluindo o fundo cinza da mensagem do usuário; respeita `NO_COLOR` e mantém cursor sem blink. `autocomplete.go` oferece completions para comandos, skills e arquivos `@path`.
- `internal/ui/login.go` usa o list do Bubbles e oferece teclado para escolher provider e opção de persistência. `internal/cli/onboarding.go` já usa Huh; `huh/v2` é dependência direta existente.
- `internal/app/types.go` fornece os limites da UI: `Event` contém run/session/sequence/type/text/tool/data; `Result` contém estado, exit code, checkpoint e aprovação; `Approval` contém argumentos redigidos, caminho, diff, fingerprint, IDs e agente.
- `internal/app/service.go` encaminha `agent.EventEnvelope` ao chat. O payload JSON já inclui `tool_call`, `tool_result`, `tool_progress`, `text_delta` e `thinking_delta`; o tipo do evento é preservado. Eventos do Y incluem `message_delta`, `thinking_delta`, `tool_started`, `tool_progress`, `tool_completed`, estados, retry e compaction. Output de tool pode ser arquivado/truncado pela política da app; não reconstruir saída completa a partir do texto truncado.
- `internal/app/approval.go` produz um preview de `write_file` e um texto genérico de revisão para `edit`/`patch`; `Guard` verifica fingerprint e rejeita aprovação caso o arquivo mude. Renderer não pode alterar nem perder o ID/fingerprint usado pelo fluxo de aprovação.
- `internal/ui/errors.go` já resume erros e guarda diagnóstico integral para `/details`. `stateLabel` reconhece `waiting_approval`, mas o serviço retorna `awaiting_approval`; a normalização de estados merece correção dentro da fatia de feedback.
- `go.mod` já fixa versões de Bubble Tea, Bubbles, Huh, Lip Gloss e Harmonica. Glamour não aparece em `go.mod`/`go.sum`; a fatia de Markdown deverá avaliá-lo e adicioná-lo com versão exata se a renderização incremental e as opções de estilo forem adequadas.
- A suíte da UI contém cobertura para clipping/resizing, Unicode, `NO_COLOR`, `--no-motion`, layout em dimensões reduzidas, seletor de modelos, commits deduplicados, cancelamento com executor cancelável e contratos de geração de preview inline. Ela passou na validação final.

## Decisão arquitetural antes de codificar

### Um ledger de apresentação; sem mensagens duplicadas

Adotar um ledger efêmero de mensagens/eventos na UI, separado da persistência de execução. O ledger existe apenas para o que o processo de chat observou; carregamento do transcript histórico e picker de sessões ficam fora do P0. O checkpoint/sequence/run ID existente dá identidade para deduplicação; tool calls agrupam por `ToolCallID` quando disponível.

Tipos visuais mínimos a considerar: mensagem de usuário; resposta do assistant (parcial/final); bloco de ferramenta (call/progress/result/error); approval pendente; erro/resumo de execução. Reasoning só pode aparecer se `thinking_delta` for entregue com autorização já existente na execução e deve ser opcional/recolhível; não criar conteúdo. Eventos desconhecidos permanecem ignorados ou informativos, nunca viram texto cru sem sanitização.

### Resolver `tea.Println` vs viewport antes da fatia de viewport

O chat não pode continuar fazendo `tea.Println` de cada mensagem e também renderizar o mesmo ledger no viewport. Isso duplicaria conversa e scrollback, como o teste atual obriga a preservar. Para o requisito de histórico navegável, preferir um viewport de conversa com `bubbles/viewport` e o composer/status em layout inline, renderizando cada mensagem apenas no ledger. Remover os commits de mensagens da saída dinâmica quando o ledger assumir autoria visual. **Não ativar automaticamente `AltScreen`:** testar a compatibilidade da vista inline existente com scroll de histórico e restaurar o terminal ao sair; apenas mudar para alternate screen se a prova de UX/terminal demonstrar que inline não suporta o contrato. Se inline não suportar viewport navegável sem interferir com o scrollback do terminal, documentar e decidir o modo antes de avançar.

Metadados (banner inicial, URLs de OAuth) podem continuar usando scrollback se não forem também gravados no ledger. Definir quais mensagens pertencem a cada superfície e validar esse limite numa sessão completa. A regra é cada item de conversa ter exatamente um destino.

### Projeção de estado, não segunda máquina de execução

Criar estado visual transitório derivado de `Execution`, `Event.Type`, `Result.State`, `ExitCode`, `Error` e `Approval`. Não persistir estado visual nem permitir que uma animação indique progresso percentual inexistente. Estados candidatos: idle, starting, streaming, tool-running, approval-pending, cancelling, retryable-error, failed e in-doubt. `in_doubt`/exit code 4 é orientação de reconciliação, nunca ação retry direta. `/approve`, `/deny`, `/retry`, `/steer` e cancelamento continuam usando comandos existentes.

## Bibliotecas e responsabilidades

| Biblioteca | Uso nesta fase | Limite |
|---|---|---|
| Bubble Tea v2 | Fonte única de mutação em `Update`; recebimento ordenado das mensagens/updates; renderização e comandos assíncronos; resize/cancelamento. | Nenhum I/O em `View`; evento de UI não executa tool fora do fluxo `Executor`. |
| Bubbles v2 | `viewport` para conversa e já existentes painel/list/textarea; `spinner` para atividade. Avaliar `help`/key components somente se acrescentarem a hints existentes sem conflito. | Um viewport único para histórico da conversa; não renderizar via scrollback em paralelo. |
| Lip Gloss v2 | Composição de blocos/margens/bordas/tokens semânticos para user, assistant, ferramenta e approval. | Respeitar fundo do terminal, `NO_COLOR`, clipping já usado e dimensões Unicode. |
| Glamour | Renderização de Markdown para conteúdo assistant, com largura derivada do viewport e estilos controlados pela paleta. Não está instalado diretamente: verificar API/versionamento e acrescentar dependência exata via fluxo de dependências do repo. | Sanitizar controle ANSI externo; não assumir que saída de Glamour por si só cabe após resize; fornecer fallback plain text se estilo/color profile não estiver disponível. |
| Huh v2 | Avaliar para a decisão de approval (permitir/negar) como formulário/select integrado à experiência Bubble Tea, com ações explícitas e teclado. Reusar dep existente; não chamar `Form.Run` de forma bloqueante dentro do programa Bubble Tea. | Se Huh não puder ser embutido preservando o Update único e resize, usar `tea.KeyPressMsg`/Bubbles já integrado e registrar essa limitação; não criar duas leituras concorrentes do terminal. |
| Harmonica | Continuar usando spring para o traço de atividade já existente, apenas durante atividade real; pode animar mudança curta de estado se isso não agendar loops novos em idle. | Motion não é progresso; invalidar timer obsoleto; `--no-motion` precisa remover comandos/timers também. |

## Fatias propostas

### S0 — Contratos e spike de renderização/terminal

**Arquivos:** `internal/ui/chat.go`, `layout.go`, `theme.go`, testes existentes de UI; consulta ao código de `bubbles/viewport`, Bubble Tea e Glamour.

- Definir as estruturas de apresentação e chaves de identidade sem alterar `app.Event`, `app.Result` ou schema de persistência. Verificar quais payloads JSON chegam em `Event.Data`, incluindo IDs de tool call e dados redigidos.
- Fazer spike isolado dentro do pacote para testar Bubbles viewport no modo atual `AltScreen=false`: scroll dentro do espaço dinâmico, resize e encerramento/restore. Comparar com a opção de tela alternativa apenas se necessário.
- Verificar Glamour com texto incremental (`message_delta`) sem rerender custoso de toda conversa a cada token; escolher estilo e fallback que funcionem em `NO_COLOR`. Confirmar dependência exata compatível com Go/toolchain antes de atualizar arquivos de módulo.
- Verificar a forma de integrar Huh como submodelo Bubble Tea na etapa de approval; não pôr Huh no caminho de renderização se for um prompt concorrente/bloqueante.

**Conclui quando:** decisões acima estão escritas na revisão técnica da implementação; nenhum comportamento de aprovação/stream ainda mudou; arquitetura de scrollback tem uma opção testável escolhida. Não alterar os testes de contrato por conveniência.

### S1 — Ledger tipado de apresentação e stream idempotente

**Arquivos prováveis:** novo `internal/ui/messages.go` ou `conversation.go`; `internal/ui/chat.go`; talvez `internal/ui/errors.go`.

- Representar mensagens visuais com chave estável, papel/tipo, texto bruto sanitizado no render, status parcial/final e metadados mínimos. Guardar o conteúdo canônico antes da transformação Markdown.
- Inserir a mensagem de usuário uma vez antes do start; representar follow-up/steering de acordo com o que o evento/ação realmente aceitou, sem comunicar aceitação antes do retorno de `FollowUp`/`Steer`.
- Atualizar a mesma resposta assistant em cada delta, finalizando em `Result`; preservar a deduplicação atual por run/checkpoint e sequence, adaptando-se a eventos de run filho sem sobrescrever o texto principal.
- Mapear apenas payload conhecido de eventos. Decodificação inválida deixa a conversa íntegra e atualiza status/diagnóstico sem crash.
- Não expor argumentos/output sensíveis não redigidos; eventos do runner chegam de `distributed_events.go` com payload `tool_call` e `tool_result` redigidos. `safeText` permanece na borda de renderização.

**Critérios da fatia:** um update repetido não duplica mensagem; texto acumulado segue a ordem de sequência; terminal Result não perde stream se `Result.Text` for fallback; erros e cancelamentos finalizam o bloco certo; comandos preservam comportamento atual.

### S2 — Viewport de conversa e política de scrollback

**Arquivos prováveis:** `internal/ui/layout.go`, `internal/ui/chat.go`, novos `conversation.go`/`message_view.go`, `internal/ui/chat_test.go`, `layout_test.go`, testes PTY existentes.

- Substituir tail isolado de `m.text` pelo `viewport.Model` alimentado a partir do ledger; manter editor e status como regiões separadas.
- Autoscroll enquanto o usuário está no fim; quando o usuário sobe, novos deltas atualizam a resposta sem deslocá-lo. Sinalizar que chegaram itens novos e restaurar o fim via atalho explícito.
- Definir bindings sem conflito com editor/history/painéis: PageUp/PageDown/Home/End quando viewport focado; decidir alternância foco editor/conversa e expor hints. Não sequestrar teclas que o editor já usa sem revisão.
- No resize, recalcular altura/largura e wrapping do conteúdo; manter posição/âncora visual de mensagem, não apenas um índice de linha antigo, pois Markdown pode recalcular o número de linhas.
- Remover `tea.Println` de mensagens que agora estão no ledger. Manter apenas os textos de saída (p.ex. URL OAuth) que não fazem parte do chat. Atualizar o teste `TestInlineViewStaysBoundedDuringLongUnicodeStreamAndResize`, cuja expectativa atual exige scrollback final e contradiz o novo requisito; criar novo contrato para ausência de duplicação e retenção visual.
- Considerar cache de render limitado e/ou renderização lazy; truncamento deve ser indicado sem apagar conteúdo canônico. Tool output mantém limite da app e pode apontar para `/details`/inspect.

**Critérios da fatia:** histórico da sessão de processo navegável; stream aparece num único lugar; composer segue editável; sem overflow de largura/altura; terminal restaura ao terminar nos sistemas suportados.

### S3 — Markdown incremental, largura e temas de conteúdo

**Arquivos prováveis:** novo `internal/ui/markdown.go`; `internal/ui/layout.go`, `theme.go`, `chat.go`; atualizar `go.mod`/`go.sum` se Glamour aprovado.

- Renderizar blocos de texto do assistant pelo Glamour com largura do viewport e estilos mapeados aos tokens de Opera. Renderizar texto do usuário sem assumir que foi escrito em Markdown, a menos que esse seja um contrato decidido.
- Enquanto o assistant está transmitindo um fence/código incompleto, renderizar de forma estável e tolerante; no evento final, produzir o mesmo conteúdo final sem duplicar prefixo.
- O wrapping e limites são por largura terminal/célula visível, não bytes. Preservar wide chars/Unicode. A sanitização atual `safeText` deve aplicar antes de transformar; nunca interpretar ANSI recebido como markup confiável.
- `NO_COLOR` deve desabilitar todo estilo gerado, também no Glamour; oferecer fallback de texto plano. Cachear render somente por conteúdo + largura + perfil/tema; invalidar em resize.

**Critérios da fatia:** parágrafos, listas, links/code fences e código legíveis; teste sem cor e resize; nenhum ANSI externo passa para o terminal; resposta em stream não causa travamento/perda de sufixo.

### S4 — Blocos de tool e ciclo de operação

**Arquivos prováveis:** novo `internal/ui/tool_message.go`; `chat.go`, `layout.go`; tipos visuais definidos em S1.

- Decodificar `tool_call`, `tool_progress` e `tool_result` de `Event.Data`; agrupar por `ToolCallID` ou correlation/idempotency key existente. Usar `Event.Type` como discriminador, não inferir todas as transições por `Tool != ""`.
- Mostrar nome, caminho/argumentos de forma resumida e redigida, estado em execução/concluído/falhou e output efetivamente disponibilizado. Não transformar todo conteúdo arbitrário em Markdown; tool output é texto delimitado/sanitizado, com estado truncado explícito quando metadata indicar truncamento.
- Manter a resposta do assistant e as tools como itens distintos; status global resume qual ação está ativa. Eventos repetidos/ordenados por sequência atualizam o item, não criam novos.
- Para saída grande, exibir excerpt com affordance de consulta ao caminho de inspeção já existente (`/sessions … output`/`/details`, conforme interface alcançável) ou deixar claro que UI só mostra trecho. Não prometer output que `OutputLimit`/archive não entregou.

**Critérios da fatia:** tool call, progress, resultado e erro correspondem à mesma chamada; nenhum argumento secreto é exposto; progresso visual não mente; stream de texto assistant permanece íntegro junto dos blocos.

### S5 — Approval e diff legível

**Arquivos prováveis:** novo `internal/ui/diff.go` e `approval_view.go`; `chat.go`, `layout.go`, possivelmente `login.go`/componente Huh separado; `internal/ui/*_test.go`.

- Mostrar o objeto `app.Approval` associado à ferramenta pendente; destacar ferramenta, paths, agente/tarefa, ID e consequência. Argumentos já vêm redigidos.
- Renderizar `Approval.Diff` como diff somente quando for formato de diff real. `write_file` fornece `---/+++` e linhas +-; `edit`/`patch` atualmente recebem texto “Current file … Requested edit …”, portanto precisam permanecer revisão de texto/argumentos sem colori-los falsamente como patch. Não inventar diff que backend não conhece.
- Resumir diff grande com limite de linhas e indicador de corte, manter o dado para revisão acessível/scroll; garantir largura Unicode e `NO_COLOR`.
- Oferecer ações focáveis Allow/Deny, preferencialmente via modelo Huh integrável ao Bubble Tea, além dos comandos `/approve` e `/deny` existentes. Mover o foco para diálogo somente após existir approval; Escape fecha detalhes ou retorna para o pedido conforme definido, sem consumir aprovação.
- A ação deve enviar somente o ID de aprovação vinculado a `m.resume`; aplicação revalida ID/fingerprint por `Guard`, portanto a UI não cria nova autoridade nem armazena um booleano de aprovação sem ID.
- Se uma revisão nova vier após `edit/patch`, atualizar a apresentação sem permitir que decisão anterior seja reutilizada. Result continuado pode produzir outro checkpoint e approval.

**Critérios da fatia:** terminal confirma/negar sem digitação de comando e ainda mantém slash aliases; mudança do arquivo durante review resulta na rejeição/novo pedido pelo fluxo existente; deny não executa efeito; diff textual nunca é apresentado como patch válido.

### S6 — Estados de atividade, cancelamento e erro acionável

**Arquivos prováveis:** `internal/ui/errors.go`, `chat.go`, `layout.go`, `theme.go` e testes de estados.

- Derivar status visual dos valores reais. Cobrir diferença `awaiting_approval` vs `waiting_approval`, `completed`, `failed`, `cancelled/canceled`, `in_doubt`, estado de `Execution` e tipo `tool_*`.
- Separar status persistente curto (linha de status), erro acionável em conversa e diagnóstico integral sob `/details`. Erro resumo não deve perder run/checkpoint necessários para retry seguro.
- `retryPrevious` verdadeiro só nos estados realmente retomáveis e sem approval pendente; `in_doubt` dirige para reconciliação existente, não para retry.
- Manter spinner Bubbles e Harmonica apenas enquanto `stream`/login/ação assíncrona relevante estiver ativo; um timer por geração, cancelar visualmente no resultado/erro/saída e nenhum tick em idle.
- Cancelamento mostra “cancelando” enquanto drena resposta; após fechamento, apresentar o estado real `cancelled`/recoverable. Não dizer que houve cancelamento concluído antes do Result.
- Transformar `/details`/status em affordances com texto curto e hints contextuais; manter texto útil com `NO_COLOR`, `--no-motion` e largura pequena.

**Critérios da fatia:** todos os estados têm texto e próxima ação corretos; sem retry de `in_doubt`; nenhum spinner órfão nem loop de redraw idle; errors repetidos dedupados.

### S7 — Integração e aceite manual/PTy

**Arquivos:** testes atuais em `internal/ui` e fixtures de terminal; documentação do P0 só se o comportamento externo mudar.

Validar o conjunto usando as suítes e fixture existentes (sem rodá-las durante esta etapa de planejamento):

- longo streaming assistant, tool longa com progress/result e error, follow-up durante stream, approval/deny/reapproval e resume;
- eventos fora de ordem/replay, execução filho, Result sem deltas e Result duplicado;
- terminal 120×30, 80×24, 40×12, 20×8, 5×3; resize acima/abaixo durante stream e enquanto usuário está scrollando;
- texto Unicode largo, Markdown incompleto/final, diff grande, output truncado, argumento redigido;
- `NO_COLOR`, `--no-motion`, acessível, sem TTY dentro dos fluxos já suportados, Ctrl+C e restauração terminal;
- nenhuma mensagem da conversa duplicada entre viewport e terminal scrollback; URLs de autenticação permanecem observáveis; fluxo de saída compatível nos PTYs Unix/Windows.

## Dependências e ordem

`S0 → S1 → S2 → S3`; `S4` depende de S1 e pode ocorrer em paralelo à conclusão do renderer S3 somente se ambos usarem o contrato tipado de mensagens; `S5` depende de S1 e da apresentação de tool de S4; `S6` começa com S1 e fecha após S4/S5; `S7` aceita o conjunto completo.

Não fazer mudança de backend Y, schema durable, carregamento de sessão anterior, tree picker, settings, tema selecionável, imagem inline, extensão de UI nem novos comandos de navegação: são escopo P1/P2. Se payload disponível não bastar para uma visualização, mostrar só o dado real e registrar gap para etapa separada em vez de forçar mudanças de contrato no P0.

## Riscos a acompanhar

| Risco | Resposta no desenho |
|---|---|
| Conteúdo duplicado entre viewport e scrollback | Ledger único para mensagens; `tea.Println` reservado a saída que não é conversa; fixture PTY confirma saída uma vez. |
| Bubbles viewport muda a semântica da TUI inline | Spike S0 antes de converter testes; não misturar `AltScreen` com scrollback sem decisão e teste de terminal real. |
| Re-render Glamour a cada delta é caro ou instável | Atualização da mensagem existente; limitar cache a viewport/versão; medir com stream longo; fallback plain text se renderer for lento. |
| Resize muda wrap e altura de todo o histórico | Recalcular layout/cache por largura e ancorar por mensagem/offset, não linha absoluta; clipping final mantido. |
| Approval desassociada da operação ou diff ambíguo | Vincular a ID/fingerprint, distinguir diff formal de resumo textual, permitir nova aprovação após mismatch. |
| Payload de evento muda ou vem parcial | Tratar `Type` + `Data` tipados pelo schema público; decoder tolerante; sem render de JSON cru como conteúdo. |
| Resposta/tool output volumosos aumentam memória | Render lazy/cache bounded; output truncado é explícito; não remover a única cópia visual sem indicação e manter inspeção da camada app. |
| Huh cria loop de entrada separado | Usar somente componente/modelo embutível no Bubble Tea; decisão explícita se não houver integração segura. |
| Compatibilidade do Glamour/paleta diverge de `NO_COLOR`/color profile | Dependency review + fixture de saída sem cor e estilo; pin exato, sem adivinhar API. |

## Critério de conclusão do P0

Marcar P0 de `.agents/plans/ui/README.md` concluído somente quando S0–S7 estiverem aceitas: conversa navegável sem duplicação; Markdown e diff acessíveis; tools e approval rastreáveis; estados de operação/recovery acionáveis; streaming, resize, Unicode, modo acessível, `NO_COLOR` e `--no-motion` coerentes; terminal restaurado em PTY; e a UI continuar consumindo somente updates/commands existentes do Opera.
