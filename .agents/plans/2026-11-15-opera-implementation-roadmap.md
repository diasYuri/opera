# Opera — roadmap de implementação

Estado: F0–F4 encerradas; implementação da F5 e validação local concluídas. Aceite final multiplataforma aguarda execução dos jobs nativos. SDK Y v1.3.4 publicado sem replace; correções de aprovação/saída estruturada e licença integradas em y#4 (merge `aa1c4b1c01734898d1b28ca6b6cc884dde5bdcde`, 2026-11-16). Nenhuma publicação do Opera.
Data da análise: 2026-11-15.
Destino: `github.com/diasYuri/opera`, binário `opera`.
Workspace: `/Users/yuri/git/diasYuri/opera`.

## 1. Resultado esperado e limites

Entregar um harness CLI de coding com conversa no scrollback, ferramentas, aprovações, planejamento, subagentes locais, sessões e retomada explícita. `chat`, `run` e `worker` usarão o mesmo serviço de execução e os mesmos contratos de autorização, ferramentas e persistência.

Toda execução pertence ao processo do CLI. Checkpoint preserva dados para uma futura retomada; não mantém tarefas rodando. Fechar o terminal encerra a árvore de execução. O produto final deve estar implementado, documentado, testado e empacotável.

Ficam fora: daemon, workers remotos, jobs destacados, worktrees automáticas e publicação/distribuição pública do Opera. A publicação de uma versão do **SDK Y**, necessária para consumir as APIs novas, é uma dependência desta entrega e não equivale a publicar o Opera.

Este documento de planejamento está em português conforme a solicitação. Interface, erros apresentados ao usuário, ajuda, exemplos e documentação do produto serão em inglês. Os agentes respondem no idioma da conversa.

## 2. Base observada e verificação de dependências

O diretório do Opera estava vazio, sem `go.mod`, Git ou implementação. O Y foi consultado em `~/git/diasYuri/y` (`/Users/yuri/git/diasYuri/y`), HEAD `d0d1ca36fab745e3fae6c25287c623c08ea064f1`, sem alterações locais informadas por `git status --short`. Essa revisão serve apenas como referência inicial; a versão publicada escolhida na fase 0 será a autoridade da integração.

O Y já contém `pkg/runtime.Harness`, contratos de componentes/scopes, `AgentRunner`, stores abstratos, `RuntimeView`, sessões, ferramentas, providers, contexto, LSP, memória, MCP e WASM. Sua documentação orienta consumidores a escrever factories próprias sobre contratos públicos, pois a composição padrão está em `internal/runtime`. O `go.mod` observado usa Go 1.25.0 e já depende de SQLite puro Go (`modernc.org/sqlite`). Isso não comprova que todos os adapters necessários sejam públicos ou que a integração esteja pronta.

Referências locais para a implementação:

- `../y/docs/runtime-composition.md` e `../y/examples/component-harness`: composição pública.
- `../y/docs/component-contract.md`: dependências, serviços e lifecycle.
- `../y/docs/effects-and-durable-state.md`: efeitos, persistência e reconciliação.
- `../y/docs/runtime-protocol.md` e `../y/docs/wasm-components.md`: contratos opcionais.
- `../y/pkg/agent/distributed_runner.go`, `distributed_run.go`, `runtime_view.go` e `runtime_view_acquire.go`: execução durável e views.
- `../y/internal/auth`, `internal/storage`, `pkg/providers/auth`, `pkg/session`, `pkg/extensions/task-manager` e `pkg/memory`: pontos para a auditoria das lacunas públicas.

### 2.1 Autorização para alterações no Y

Está explicitamente autorizado alterar o código do Y em `~/git/diasYuri/y` (`/Users/yuri/git/diasYuri/y`) sempre que necessário para implementar este roadmap, incluindo APIs públicas, adapters, correções, testes, exemplos e documentação. Essas alterações fazem parte do trabalho do Opera e não exigem nova confirmação para cada mudança necessária. Respeitar as instruções do repositório Y, preservar alterações locais existentes e manter as mudanças compatíveis e testadas. O Opera continua consumindo somente APIs públicas; a entrega final depende de versão publicada do SDK, sem `replace` local permanente.

### 2.2 Versões obrigatórias

As releases abaixo foram localizadas em fontes oficiais. A fase 0 ainda deve verificar `module` de cada tag, resolução conjunta, checksums e compilação; existência de release não garante compatibilidade entre todas elas.

| Dependência | Versão fixa | Responsabilidade | Fonte oficial |
| --- | --- | --- | --- |
| Go | 1.27.4 (última estável da série 1.27 verificada em 2026-11-15) | Desenvolvimento e CI | [Downloads oficiais](https://go.dev/dl/?mode=json) |
| Bubble Tea | v2.1.1 | Loop da interface | [Release](https://github.com/charmbracelet/bubbletea/releases/tag/v2.1.1) |
| Bubbles | v2.2.1 | Editor e componentes | [Release](https://github.com/charmbracelet/bubbles/releases/tag/v2.2.1) |
| Huh | v2.0.3 | Formulários | [Release](https://github.com/charmbracelet/huh/releases/tag/v2.0.3) |
| Lip Gloss | v2.1.1 | Layout e tema | [Release](https://github.com/charmbracelet/lipgloss/releases/tag/v2.1.1) |
| Harmonica | v0.2.0 | Animações discretas | [Release](https://github.com/charmbracelet/harmonica/releases/tag/v0.2.0) |

Usar a última release estável de Go 1.27.x. A versão verificada em 2026-11-15 é **Go 1.27.4**, conforme o [catálogo oficial](https://go.dev/dl/?mode=json). Ao iniciar a implementação, verificar novamente os patches estáveis da série 1.27 e, se houver versão mais recente, atualizar e fixar o mesmo patch no ambiente de desenvolvimento, módulo, CI e empacotamento. Não usar beta/RC nem avançar para outra série sem decisão explícita. Validar também as alterações do Y com essa toolchain; alterar seus requisitos de módulo/CI quando necessário para compatibilidade.

Fixar versões e caminhos reais em `go.mod`/`go.sum`; não inferir caminhos de módulo a partir das URLs GitHub. Fixar também TOML, lint, empacotamento e ferramentas de teste escolhidas. O patch estável de Go 1.27.x selecionado (inicialmente 1.27.4) deve ser usado efetivamente, não apenas declarado como mínimo: configuração do ambiente e CI verificam `go version` e impedem atualização implícita de toolchain. Todos os builds do produto usam `CGO_ENABLED=0`.

Se qualquer versão não resolver ou houver incompatibilidade, registrar o bloqueio e uma decisão explícita. Não substituir silenciosamente as versões exigidas.

## 3. Invariantes de arquitetura

1. Importar somente contratos públicos do Y. Proibidos `Y/internal`, backend via binário `y` e cópia do loop de agentes.
2. Compor com `runtime.NewHarness` e factories do Opera. Prompts, perfis, defaults, comandos e identidade visual pertencem ao Opera.
3. Centralizar execução em um serviço de aplicação. Adapters de CLI/UI não criam providers nem controlam goroutines do agente.
4. I/O ocorre fora de `Update`/`View`. Eventos chegam como mensagens Bubble Tea; somente o loop altera o modelo da UI.
5. Providers, processos, inscrições, views e filhos têm ownership explícito em scopes. Encerrar dependentes antes de dependências; tentar todos os cleanups e agregar erros.
6. O caminho efetivo de ferramentas, inclusive snapshots de `RuntimeView`, preserva política, aprovação, identidade e `OperationStore`.
7. Toda mutação passa pelo coordenador do workspace canônico. Ferramentas externas e shell são mutantes por padrão.
8. Estado durável não é apagado por cleanup. Shutdown não promete desfazer efeitos externos.
9. Falhas de persistência não podem transformar execução durável em execução silenciosamente efêmera.
10. Segredos não entram em TOML versionável, eventos, logs, checkpoints, exportações ou prompts de diagnóstico.

### 3.1 Estrutura proposta

Os nomes abaixo são uma proposta do Opera, não APIs já existentes no Y.

```text
cmd/opera/                 entrypoint e exit codes
internal/cli/              parsing, comandos, formatos e detecção de TTY
internal/app/              casos de uso e serviço único de execução
internal/harness/          factories, bindings e composição pública do Y
internal/workspace/        identidade canônica e coordenação de ferramentas
internal/approval/         fila, apresentação e resolução de operações
internal/config/           TOML, precedência, proveniência e validação
internal/credentials/      integração com store restrito de segredos
internal/persistence/      wiring dos adapters públicos e metadados Opera
internal/context/          AGENTS.md, arquivos e anexos
internal/ui/               modelo Bubble Tea, eventos e componentes
internal/output/           texto, JSONL e redação de dados sensíveis
internal/diagnostics/      capacidades, dependências e gerações
docs/ examples/ testdata/  documentação em inglês e fixtures
```

Adapters genéricos de persistência e OAuth pertencem ao Y. `internal/persistence` do Opera os configura e acrescenta somente metadados do produto; não recria uma implementação concorrente dos stores do SDK.

### 3.2 Serviço de execução e eventos

- Definir contratos pequenos para iniciar/retomar execução, enviar follow-up, steering, resolver aprovação e cancelar; nomes finais dependem das APIs publicadas do Y.
- Separar sessão, branch, run, task, agent e operation por IDs estáveis. Uma sessão pode ter múltiplos runs; uma operação pertence a uma execução e a um agente identificável.
- Envelope de eventos versionado com sequência, timestamp, IDs de correlação, tipo e payload. Preservar ordenação por execução e documentar a ordenação entre filhos.
- Eventos duráveis registram transições relevantes. Deltas visuais podem ser agregados sem perder texto final, aprovações, resultados ou término.
- Filas limitadas, backpressure e cancelamento explícitos; nenhuma goroutine por token. Consumidor lento não pode perder eventos críticos.
- Estado de execução distinto do estado de operações: idle, running, awaiting approval, cancelling, completed, failed, cancelled e interrupted; operações podem estar pending, in-flight, completed, denied ou in-doubt conforme mapeamento dos estados reais do Y.
- Steering e follow-up são ações distintas. Mensagem comum durante execução vira follow-up ordenado; não altera silenciosamente o turno em andamento.

## 4. Caminho crítico e regras de conclusão

Sequência: **F0 inventário/bootstrap → F1 APIs do Y + versão publicada + prova vertical → F2 coding interativo → F3 sessões/workers → F4 extensões/inspeção → F5 fechamento**.

A persistência mínima e a segurança nascem na prova vertical; F3 amplia esses contratos. Não adiar checkpoint, autorização e reconciliação para depois de implementar ferramentas com efeitos.

Cada item usa `[ ]` até existir implementação e evidência. Uma fase só termina quando o fluxo demonstrável funciona e suas verificações passam. Trabalho de uma fase posterior pode ser preparado, mas não serve para dispensar um gate anterior. Não fixar datas antes de medir as lacunas do SDK.

## 5. F0 — inventário, decisões e bootstrap

**Objetivo:** transformar as premissas em contratos verificáveis e criar a base mínima do módulo.

- [x] F0.1 Inicializar Git e módulo `github.com/diasYuri/opera`, `cmd/opera` e scaffolding mínimo; preservar as instruções locais.
- [x] F0.2 Fixar e resolver as versões da seção 2.2; verificar e fixar o patch estável mais recente de Go 1.27.x; registrar caminhos de módulo, checksums e teste de compilação conjunta sem cgo.
- [x] F0.3 Selecionar versão-base publicada do Y e comparar com a revisão local observada. Inventariar exports, providers, ferramentas, recursos opcionais e build tags dessa versão.
- [x] F0.4 Criar `docs/y-capability-coverage.md` com linhas por capacidade pública aplicável, símbolos/pacotes, feature flags, modos consumidores e testes. A seção 10 deste roadmap é a semente, não um inventário exaustivo certificado.
- [x] F0.5 Produzir decisões curtas em `docs/adr/`: composição/lifecycle, eventos, persistência/recovery, política/fila, terminal e credenciais.
- [x] F0.6 Definir paths multiplataforma de config/data/cache e workspace, precedência, schema TOML, formato JSONL e exit codes.
- [x] F0.7 Implementar configuração com proveniência por chave e store separado de segredos. Decidir keyring quando disponível e fallback restrito explícito; validar permissões Unix e ACLs Windows.
- [x] F0.8 Introduzir CI mínima de formatação, vet, testes e build Linux/macOS/Windows com Go exato. Adicionar lint configurado e fixado antes do fechamento da primeira fase.
- [x] F0.9 Definir orçamento de contexto, limites de saída/processo/anexo, timeout de shutdown e políticas para ausência de ferramentas externas.

**Aceite:** `opera --help`, `--version`, leitura de configuração e `doctor` básico funcionam; dependências resolvem em ambiente limpo sem `replace`; inventário e gaps do Y têm responsáveis por componente e testes previstos. Não exigir OAuth/modelo real para help, version e diagnóstico local.

### Evidências de conclusão da F0 (2026-11-15)

- Implementação: `cmd/opera`, configuração com proveniência, stores de credenciais e `doctor` local; Git inicializado em `main`.
- Dependências: Go 1.27.4 e versões Charm exigidas fixadas; Y publicado v1.2.0 no mesmo commit da revisão auditada, sem alterações necessárias no SDK nesta fase.
- [Relatório de validação](../../docs/f0-validation.md): checks, lint, race local, seis alvos de compilação cruzada, smoke do binário e resolução/testes com cache público de módulos vazio.
- [Inventário e cobertura do Y](../../docs/y-capability-coverage.md), [decisões de arquitetura](../../docs/adr/README.md), [contrato CLI/JSONL](../../docs/cli-contract.md) e [configuração/limites](../../docs/configuration.md).
- Linux/Windows têm execução nativa configurada na CI; não há alegação de execução remota ou de validação de ACL Windows em macOS. Os gaps Y1–Y6 e toda execução de agentes continuam no escopo da F1 e fases seguintes.

## 6. F1 — APIs públicas do Y e prova vertical

**Objetivo:** executar um prompt real com streaming, ferramenta, aprovação, checkpoint e retomada antes de ampliar a interface.

### 6.1 Alterações aditivas no Y

| ID | Trabalho no Y | Contrato e evidência exigidos |
| --- | --- | --- |
| Y1 | OAuth reutilizável | API pública com HTTP client, configuração, relógio/espera testáveis e callbacks de URL/device code; sem imprimir, ler terminal ou abrir browser por conta própria. Testar fluxos suportados, refresh, expiração, recusa e cancelamento com HTTP fixtures. |
| Y2 | Adapter SQLite público | Implementar os contratos necessários para checkpoints, eventos, operações, transcritos, sessões, branches e metadados. Exportar/adaptar a implementação reutilizável sem dependência de `internal`. Migrações, transações, concorrência, readiness e recuperação testadas com cgo desativado. |
| Y3 | Adapter filesystem de memória | Store público com paths injetáveis, atomicidade, readiness, versão e erro de escrita observável. Cleanup preserva dados; esquecer memória é operação explícita. |
| Y4 | Snapshot/restore do task manager | Preservar IDs, parentesco, status, dependências e ordenação necessária; validar versão, referências ausentes e hierarquias inválidas. Round-trip sem regenerar identidades. |
| Y5 | Execução ativa no AgentRunner | Eventos públicos e controle de steering, follow-ups e cancelamento sem acessar `Agent` privado. Definir ownership, concorrência, backpressure, conclusão, rejeição após término e isolamento entre runs. |
| Y6 | Operações duráveis com RuntimeView | Garantir que registry capturado continue dentro do controle de operações. Integração deve observar Begin/Complete no store, resultado deduplicado e bloqueio de replay de efeito incerto; testar também approval/deny e troca de geração. |

- [x] F1.1 Auditar o que já existe antes de adicionar Y1–Y6; nomear os contratos finais em ADR e atualizar a matriz.
- [x] F1.2 Implementar e testar Y1–Y6 diretamente em `~/git/diasYuri/y`, conforme a autorização da seção 2.1, com mudanças compatíveis; manter independência de `pkg` em relação a `internal` e atualizar exemplos públicos.
- [x] F1.3 Executar checks de arquitetura e suites default/feature-tagged afetadas do Y. Criar teste externo ao módulo, como consumidor do SDK, para comprovar acessibilidade pública.
- [x] F1.4 Gerar changelog/documentação e obter uma versão publicada do SDK com os contratos. Consumir tag fixa no Opera e provar instalação/build fora do checkout do Y.

Desenvolvimento coordenado pode usar `go.work` temporário fora da entrega; nenhum `replace` local permanente nem dependência de paths vizinhos pode chegar ao gate. A liberação da versão do Y é bloqueio explícito se não estiver disponível; não contornar com import privado.

### 6.2 Integração mínima do Opera

- [x] F1.5 Compor provider, registry, política, stores, runner e serviço de aplicação via `runtime.Harness`; scopes cobrem inicialização parcial e encerramento.
- [x] F1.6 Onboarding TTY com Huh: provider/modelo e política read-only/ask/trusted; destacar `ask` sem efetivar confiança silenciosa.
- [x] F1.7 Integrar OAuth/credencial com callbacks apresentados pela UI; nenhuma dependência do SDK em stdin/stdout.
- [x] F1.8 Implementar `opera run`, `resume <id>` e chat mínimo sobre o mesmo serviço, com streaming e uma ferramenta de leitura e uma de mutação controlada em fixture.
- [x] F1.9 Implementar aprovação vinculada à operação, negação, persistência antes de efeitos, cancelamento e retomada de aprovação pendente.
- [x] F1.10 Criar renderização mínima sem alternate screen: bloco final é emitido uma vez; streaming/status permanecem dinâmicos. Validar a API real do Bubble Tea fixado.
- [x] F1.11 Implementar modo sem TTY em texto/JSONL, sem formulários, com estado durável e exit code para aprovação pendente.
- [x] F1.12 Introduzir fila exclusiva de mutações do workspace desde a primeira ferramenta mutante; ampliar concorrência em F3.

**Fluxo demonstrável:** escolher modelo/política → enviar prompt → receber streaming → solicitar mutação → aprovar/negar → salvar checkpoint → encerrar → `resume` → concluir sem duplicar efeito.

**Aceite:** integração fake determinística cobre o fluxo inteiro; smoke real opt-in registra apenas metadados redigidos. Gate exige versão publicada do Y, teste de RuntimeView/OperationStore, limpeza de recursos, interrupção recuperável e nenhuma importação privada.

### Evidências de conclusão da F1 (2026-11-15)

- [Relatório F1](../../docs/f1-validation.md): prova vertical determinística e real com OpenAI gpt-5-mini, incluindo aprovação, fechamento, retomada em outro harness, escrita/leitura durável e checkpoint final 7.
- SDK publicado `github.com/diasYuri/y v1.3.1`, commit `b25d3450185f58573c8ddb4e62e82391c78000b9`, com Y1–Y6 e correção de streaming encontrada no smoke real. [APIs integradas em y#1](https://github.com/diasYuri/y/pull/1); [patch de streaming em revisão em y#2](https://github.com/diasYuri/y/pull/2).
- `chat`, `run` e `resume` compartilham serviço de aplicação, composição pública, política, stores e fila. Onboarding e decisões de confiança ficam em configurações de usuário por workspace, fora do projeto.
- Consumidor independente em `testdata/sdk-consumer`, módulo final sem `replace` ou workspace de integração, testes HTTP/PTY, cancelamento, reconciliação explícita e cleanup em ordem de dependência.
- CI nativa do SDK passou em Linux/macOS/Windows no PR #1, já integrado. O patch #2 permanece em revisão. Para o Opera, as seis compilações cruzadas passaram; CI nativa está configurada, sem execução remota. O Opera permanece local, sem publicação.

## 7. F2 — coding interativo

**Objetivo:** tornar `opera` e `opera chat` utilizáveis para trabalho diário no workspace.

- [x] F2.1 Editor multiline com Enter para enviar, Alt+Enter para newline, histórico, paste seguro e navegação. Documentar fallback para terminais que não distinguem a combinação.
- [x] F2.2 Implementar protocolo de scrollback: blocos com identidade/sequência, commit único, região dinâmica limitada e resize sem reemissão do histórico. Testar separadamente replay de sessão e novas mensagens.
- [x] F2.3 Seletores, forms, ajuda, diff e detalhes temporários com altura limitada. Bubbles fornece componentes; Huh fica restrito aos fluxos de formulário coordenados pelo adapter de UI.
- [x] F2.4 Contexto por arquivos e AGENTS.md com escopo e ordem documentados; respeitar regras aplicáveis, limites, ignore e referências de origem. Conteúdo de arquivo não concede permissão.
- [x] F2.5 Anexos compatíveis com as capacidades do modelo: validar tipo/tamanho, orçamento e disponibilidade antes de enviar; erro explícito para modalidade não suportada e redação no armazenamento/export.
- [x] F2.6 Seleção provider/modelo e thinking conforme suporte, incluindo troca em safe point e diagnóstico de opções incompatíveis.
- [x] F2.7 Integrar filesystem, patch/edição, shell, Git e busca públicos do Y através da mesma cadeia de autorização, operações e fila.
- [x] F2.8 Integrar LSP: detecção, processo por scope, navegação/diagnóstico e shutdown. Diagnosticar `rg`, `ast-grep` e language servers ausentes; oferecer busca textual/read de arquivos como fallback explícito quando aplicável.
- [x] F2.9 Tema claro/escuro adaptativo, acento azul, hierarquia por espaçamento, indicadores textuais e `NO_COLOR`.
- [x] F2.10 Harmonica somente em progresso/transições curtas; `--no-motion` e modo acessível sem ticks/redesenho contínuos, mantendo informação equivalente.
- [x] F2.11 Ctrl+C cancela run ativo e mantém sessão; em idle encerra. Ctrl+D com editor vazio encerra. Restaurar terminal após erro, cancelamento e falha de inicialização.
- [x] F2.12 Implementar `/model`, `/thinking`, `/context`, `/settings` e `/help` e ações CLI equivalentes onde aplicáveis; manter registro único de comandos/ajuda.

**Fluxo demonstrável:** abrir repositório → carregar instruções/arquivos → localizar símbolo com LSP ou fallback → editar com aprovação → executar teste via shell → visualizar diff → continuar conversa.

**Aceite:** fluxo funciona em tema claro/escuro e modo acessível; testes de terminal cobrem streaming longo, resize, Unicode, paste, teclas e erro; leitura/escrita/shell/Git seguem a política em chat e run.

### Evidências de conclusão da F2

- [Relatório F2](../../docs/f2-validation.md) registra comandos, contexto, anexos, políticas, builds com LSP e testes de aceite.
- Ferramentas públicas de filesystem, edit/patch, shell, Git, busca e LSP estão na mesma cadeia de autorização/fila/operações duráveis; fallback de leitura/busca é explícito.
- Editor com histórico/paste, seletores e painéis limitados; comandos compartilhados, troca em idle, tema adaptativo, NO_COLOR e progresso Harmonica sem ticks periódicos.
- Contexto com escopo, provenance, ignore e orçamento; imagens com capability gate e payload transitório, snapshots com hashes. Formatos binários adicionais são recusados explicitamente.
- Checks locais, lint, race nos pacotes alterados e builds CGO-free de seis targets. Execução nativa multiplataforma permanece na CI configurada.

## 8. F3 — sessões, workers e colaboração local

**Objetivo:** completar persistência e execução coordenada, com retomada consistente de tarefas hierárquicas.

### 8.1 Sessões e recovery

- [x] F3.1 Implementar `sessions list/inspect/export/branch` com IDs inequívocos, paginação e export redigido/versionado. Fixar a sintaxe real na ajuda.
- [x] F3.2 Expandir SQLite para todos os registros do produto; migrações versionadas com fixtures de versões anteriores, backup e falha segura em schema futuro/corrompido.
- [x] F3.3 Distinguir checkpoint, transcript e eventos; salvar estado consistente entre run, tarefas, operações e cursor de eventos. Definir fronteiras transacionais e reconstrução quando não couberem numa transação.
- [x] F3.4 Branch herda contexto/checkpoint escolhido e proveniência; gera novas identidades de execução/operação sem repetir efeitos nem reaproveitar autorizações automaticamente. Branches compartilham arquivos reais: não fornecem isolamento de workspace.
- [x] F3.5 Compaction preserva instruções aplicáveis, evidências, tarefas, referências a ferramentas e pendências. Manter rastreabilidade do resumo e fronteira resumida.
- [x] F3.6 Retry distingue falha de provider de operação com efeito incerto. Reusar resultado durável quando seguro; bloquear repetição de in-doubt até reconciliação explícita registrada.
- [x] F3.7 Retomada valida workspace, modelo, política atual, disponibilidade de ferramentas e gerações. Configuração restaurada nunca eleva confiança atual.
- [x] F3.8 Crash/recovery marca execução interrompida, restaura hierarquia e não agenda trabalho automaticamente ao abrir `sessions` ou iniciar o CLI.

### 8.2 Planejamento e subagentes

- [x] F3.9 Implementar `opera worker "task"` síncrono com planejamento/task manager sobre o mesmo serviço de chat/run; retornar resultado e estado final no foreground.
- [x] F3.10 Subagentes locais com até quatro concorrentes e profundidade um por default; limites ajustáveis e validados, com orçamento compartilhado e rastreio parent/child.
- [x] F3.11 Integrar snapshot/restore do task manager e de filhos. Persistir identidades e status; retomada explícita reconstitui trabalho necessário sem presumir processos sobreviventes.
- [x] F3.12 Implementar leituras paralelas e uma fila exclusiva por workspace canônico para ferramentas mutantes de todos os agentes. Canonicalizar symlinks e peculiaridades de paths Windows.
- [x] F3.13 Projetar aquisição cancelável, liberação em todos os erros e justiça da fila. Não manter o lock durante espera por aprovação; revalidar argumentos/precondições e obter lock antes do efeito. Se diff mudou, exigir nova aprovação.
- [x] F3.14 Cobrir caminhos alternativos: Git, LSP que edita, MCP, WASM e shell passam pelo coordenador quando mutantes. Classificação desconhecida assume mutação.
- [x] F3.15 Identificar múltiplos processos Opera no mesmo workspace: adotar lock local de ownership que recuse execução concorrente ou coordenação interprocesso cancelável. Não criar daemon e não permitir bypass por um segundo CLI.
- [x] F3.16 Fila única de aprovações identifica agente, tarefa, ferramenta, argumentos relevantes, paths e diff. Aprovação é ligada à identidade, payload e versão da operação/política, não a uma ferramenta genericamente.
- [x] F3.17 Implementar steering, follow-ups ordenados e cancelamento pelo controle público do runner. Cancelar pai cancela filhos, espera na fila e processos associados.
- [x] F3.18 Expor `/steer`, `/compact`, `/retry`, `/tasks` e `/agents`; ações devem ser inspecionáveis e produzir eventos correlacionados.

**Fluxo demonstrável:** worker planeja tarefa → dois filhos leem em paralelo → mutações disputam a fila → usuário nega/aprova operações → envia follow-up e steering → interrompe → retoma checkpoint com IDs preservados → conclui.

**Aceite:** testes concorrentes provam exclusão de escrita e cancelamento na espera; hierarquia e branches sobrevivem à retomada; efeitos in-doubt nunca são repetidos automaticamente. Documentar que serialização não elimina conflitos lógicos entre agentes e que as ferramentas não constituem sandbox de sistema operacional.

### Evidências de conclusão da F3 (2026-11-16)

- [Relatório F3](../../docs/f3-validation.md) registra sessões/branches, compaction, controles, workers, limites e fronteiras de recuperação.
- Dois filhos atravessam leitura paralela, fila de escrita, aprovação, fechamento e retomada com IDs de tarefas/filhos preservados pelo mesmo Service.Execute.
- Migração com backup, schema futuro/corrompido, export redigido, branches históricas, cancelamento, FIFO, ownership com aliases e orçamento compartilhado têm fixtures determinísticas.
- Checks locais, lint, race e seis cross-builds; a execução nativa da CI do Opera permanece configurada e sem publicação remota.

## 9. F4 — extensões e inspeção

**Objetivo:** integrar capacidades opcionais do Y sem criar caminhos paralelos de execução.

- [x] F4.1 MCP stdio e HTTP: conectar, configurar credenciais por referência, listar tools/resources/prompts, consumir essas capacidades e diagnosticar transporte/servidor ausente.
- [x] F4.2 Processos stdio e clientes HTTP em scopes; timeouts, cancelamento, desconexão e encerramento cobertos. Requests mutantes herdam fila, política e controle durável.
- [x] F4.3 Memória persistente com adapter público filesystem: recall, gravação, inspeção e exclusão autorizada; distinguir fail-open de leitura de erro de readiness/escrita.
- [x] F4.4 Componentes WASM: descoberta, schema, capabilities concedidas, load/unload e falhas de execução. Compilar host sem cgo e verificar tags necessárias no SDK.
- [x] F4.5 Inspecionar componentes, bindings, schemas, serviços, tools, fontes de contexto, hooks, gerações e persistência ativa/degradada usando contratos públicos.
- [x] F4.6 Reconfigurar somente nos safe points suportados; views em andamento mantêm a geração capturada. Rejeitar transições não suportadas, manter a geração anterior em falha e preservar vínculo das aprovações.
- [x] F4.7 Implementar `tools`, `mcp`, `extensions`, `memory` e ampliar `doctor`; expor estado desabilitado por build tag de forma distinta de falha em runtime.
- [x] F4.8 Impedir que configuração de projeto instale/inicie executável externo ou conceda capability privilegiada silenciosamente; mudança passa pela política/consentimento efetivo do usuário.
- [x] F4.9 Completar testes de adapters e integração com fixtures MCP stdio/HTTP, memória e módulos WASM pequenos, inclusive cleanup em falha parcial.

**Fluxo demonstrável:** conectar servidor MCP fixture → inspecionar prompt/recurso/tool → executar tool autorizado → gravar/recuperar memória → carregar componente WASM → inspecionar geração → reconfigurar em safe point → reiniciar CLI e validar persistência.

**Aceite:** nenhuma extensão contorna política, fila ou OperationStore; gerações e schemas são inspecionáveis; ausência de capacidade tem diagnóstico explícito. Recursos públicos aplicáveis encontrados além da matriz inicial entram no backlog antes do fechamento.

Evidências F4: [relatório](../../docs/f4-validation.md), [contrato de extensões](../../docs/extensions.md) e fixtures em `internal/cli`/`internal/extensions`. SDK v1.3.2 publicado sem replace; aprovações sobrevivem à retomada com RuntimeView. Builds padrão habilitam `feature_lsp,feature_wasm_ext`, sem cgo. Grants WASM suportados: logs e y_tools para leituras classificadas pelo caminho durável; capabilities brutas de filesystem/rede/processo/segredos são recusadas explicitamente. Memória usa escrita/forget explícitos, sem extração automática. Reload em idle suporta um toggle WASM existente; alterações de MCP/memória, diretórios, bytes/grants quentes ou múltiplos toggles exigem novo scope. CI nativa Linux/Windows permanece configurada, não executada nesta máquina.

## 10. Matriz inicial de cobertura do Y

Esta matriz relaciona famílias de capacidades ao produto. Na F0, decompor cada família em capacidades reais da versão publicada selecionada, incluindo todos os providers e ferramentas aplicáveis. Atualizar novamente após as mudanças do SDK e antes de F5.

O arquivo definitivo `docs/y-capability-coverage.md` deve conter para **cada linha**: ID, versão Y, pacote/símbolo, build tag, aplicabilidade, justificativa, comando/slash, modos chat/run/worker, política, store/fila utilizados, teste automatizado, evidência real quando necessária, limitação por provider e status. Status: `pending`, `implemented`, `verified` ou `not-applicable`. Ausência no inventário é falha de cobertura.

| ID | Família pública / origem inicial | Superfície Opera | Fase | Evidência mínima |
| --- | --- | --- | --- | --- |
| C01 | `pkg/runtime`, `pkg/component` | Composição, scopes, bindings e shutdown | F1 | Startup rollback e cleanup inverso com falhas |
| C02 | `pkg/providers`, `pkg/ai` | Provider/modelo, streaming, thinking, modalidades e capacidades | F1–F5 | Contratos por provider, erros normalizados e fixtures |
| C03 | `pkg/providers/auth` + OAuth público novo | `auth`, onboarding e refresh | F1 | HTTP fixtures, cancelamento e ausência de terminal no SDK |
| C04 | `pkg/agent` / runner | Execução, controle ativo, follow-ups e steering | F1/F3 | Paridade chat/run/worker e isolamento entre runs |
| C05 | Stores de snapshot/event/transcript/operation + SQLite novo | Resume e recuperação | F1/F3 | Atomicidade, migrations e crash injection |
| C06 | `pkg/session` | List/inspect/export/branch | F3 | Branch preserva origem sem replay de efeitos |
| C07 | `pkg/policy`, aprovação e identidade de operação | Read-only/ask/trusted | F1–F4 | Negação sem efeito e impossibilidade de elevar via projeto |
| C08 | `pkg/tools` / filesystem, patch, shell, Git e busca | Coding tools e `tools` | F2/F3 | Inventário por ferramenta, autorização e fila |
| C09 | `pkg/context`, `pkg/coding` | Instruções, arquivos, anexos e orçamento | F2/F3 | Escopo/proveniência e compaction; prompts próprios Opera |
| C10 | `pkg/lsp` | Navegação, diagnóstico e edição quando disponível | F2 | Processo fechado, ausência com fallback e mutações na fila |
| C11 | `pkg/extensions/task-manager` | Planejamento e `/tasks` | F3 | Round-trip de identidade/hierarquia |
| C12 | Subagentes públicos do Y | Worker e `/agents` | F3 | Limites, cancelamento e retomada; confirmar API no inventário |
| C13 | `pkg/extensions` / MCP | `mcp`, tools/resources/prompts | F4 | Stdio/HTTP, auth, timeout e cleanup |
| C14 | `pkg/memory`, extensão de memória e store novo | `memory` e recall | F4 | Persistência entre processos, readiness e erro de escrita |
| C15 | `pkg/extensions/wasm` | `extensions` e capabilities | F4 | Load/unload, falha, permissões e CGO=0 |
| C16 | RuntimeView, registries, hooks e gerações | Inspeção e reconfiguração | F1/F4 | OperationStore observado no caminho capturado e safe points |
| C17 | `pkg/telemetry` | `/usage`, tokens/custos e eventos | F5 | Agregação sem duplicação em filhos, branch e resume |
| C18 | `pkg/ai` / structured output | `run`/`worker` com schema | F5 | Schema válido, inválido e capacidade indisponível |
| C19 | `pkg/config` e schemas públicos | `config`, validação e proveniência | F0/F4 | Precedência Opera preservada sem importar defaults do Y |
| C20 | `pkg/rpc` e transports de `pkg/runtime` | Avaliar aplicabilidade | F0/F5 | Justificar exclusão de hosting remoto; reutilizar contratos locais aplicáveis |

`not-applicable` exige razão verificável ligada ao escopo. Servidores remotos/RPC não são automaticamente produto desta versão; suas capacidades locais úteis devem ser avaliadas separadamente. Falta de adapter público para uma capacidade aplicável gera gap no Y, não justificativa para marcar N/A. O fechamento exige zero linhas aplicáveis pendentes e links para testes reais.

## 11. F5 — accounting, acessibilidade e empacotamento

- [x] F5.1 Tokens e custos por turno, run, sessão e agente; distinguir valores conhecidos, estimados e indisponíveis. Registrar fonte/data de preços e não inventar custo quando o provider não fornece dados suficientes.
- [x] F5.2 `/usage` com agregação sem duplicar eventos reprocessados, filhos ou branches; preservar contabilização após retry/resume/compaction.
- [x] F5.3 Ferramentas, operações, decisões e eventos inspecionáveis, com filtros/IDs e export redigido. Truncamento visual não apaga dados necessários ao diagnóstico.
- [x] F5.4 Saída estruturada conforme capacidades reais do provider; validar schema e diferenciar formato dos eventos JSONL do conteúdo estruturado gerado pelo modelo.
- [x] F5.5 Auditar acessibilidade: teclado completo, contraste, texto sem cor, leitor de tela, `NO_COLOR`, no-motion e ausência de redesenho contínuo.
- [x] F5.6 Completar documentação em inglês: instalação local, quickstart, auth, configuração, políticas, comandos/slashes, modelos, ferramentas, MCP/WASM, memória, recovery, limites e troubleshooting.
- [x] F5.7 Exemplos executáveis chat/run/worker, JSONL sem TTY, aprovação pendente e retomada; todos usam o mesmo backend.
- [x] F5.8 Construir arquivos locais Linux/macOS/Windows em amd64/arm64 conforme suporte das dependências, com binário, licença, notices, checksums e metadados de versão. Se um alvo não for suportado, resolver ou registrar decisão antes do aceite; não omitir silenciosamente.
- [ ] F5.9 Testar extração, `--version`, `--help`, diagnóstico e execução fake dos pacotes em runners nativos; comprovar ausência de dependência de cgo e de checkout do Y.
- [x] F5.10 Revisar inventário contra exports, docs e feature tags da versão final do Y. Resolver gaps aplicáveis e documentar N/A; remover `replace`, `go.work` de integração e paths locais da entrega.
- [ ] F5.11 Executar o cenário final da seção 14 e guardar evidência reproduzível. Preparar artefatos locais/CI sem publicar Opera, criar release pública ou enviar a package registries.

Aceite nativo macOS arm64 em 2026-11-16: checks/lint, suítes sem cache com e sem features, race detector, consumidor externo, provider real e pacote extraído passaram. Relatório: `docs/macos-arm64-validation.md`; logs e resultados: `.agents/validation/macos-arm64-2026-11-16/`. A parcela desta plataforma de F5.9/F5.11 está validada; os itens permanecem abertos somente para os demais runners nativos e o aceite conjunto.

Implementação F5: [relatório](../../docs/f5-validation.md), [instalação/pacotes](../../docs/installation.md), [uso/schema](../../docs/usage.md), [operações](../../docs/operations.md) e [exemplos](../../examples/README.md). Pacotes dos seis alvos gerados; smoke do arquivo extraído executado em macOS arm64; suíte completa/race/lint e prova real OpenAI passaram. F5.9 e F5.11 mantêm o gate de execução nativa Linux/Windows e demais arquiteturas aberto: workflow com seis runners implementado, sem alegar resultados que ainda não existem. Esse gate não exige implementar novo backend nem publicar o Opera.

**Aceite:** produto empacotável e verificável em três sistemas, documentação completa, matriz de cobertura encerrada e cenário final aprovado.

## 12. Contratos operacionais transversais

### 12.1 Configuração e confiança

- Precedência exata: flags → `OPERA_*` → projeto → usuário → defaults. Distinguir ausente de valor vazio/false; validar conflitos e chaves desconhecidas.
- Projeto pode pedir configuração, mas não aumentar o nível de confiança autorizado pelo usuário. Redução de permissão é respeitada; elevação exige ação explícita fora da configuração versionada.
- Definir read-only como negação de mutações, ask como aprovação de operações que a exigem e trusted como autorização conforme política explícita, ainda com limites, auditoria e coordenação. Nenhuma política implica sandbox.
- `config`/`doctor` mostram valor efetivo e origem, com redação de segredos. Referências a credenciais podem aparecer; valores secretos não.
- Resolução do workspace e política ficam vinculadas ao run. Mudança de workspace não reutiliza aprovações do anterior.

### 12.2 Operações, aprovação e recovery

- Identidade estável e registro durável precedem o efeito. Vincular decisão a argumentos relevantes, paths, diff/precondições e versão de política/view conforme suportado pelo Y.
- Ordem lógica: preparar operação → avaliar política → persistir aprovação pendente/resolução → aguardar fila sem executar → revalidar → registrar início durável → executar → registrar resultado/checkpoint. Adaptar a ordem exata ao protocolo público de operações do Y e testar suas fronteiras.
- Negação não executa efeito. Cancelamento enquanto aguarda fila remove a solicitação; cancelamento após início pode produzir estado in-doubt.
- Não prometer exactly-once para shell/APIs arbitrárias. Persistência e idempotência evitam replay seguro conhecido; efeito sem confirmação exige reconciliação explícita.
- Definir ação de reconciliação inspecionável por ID, com confirmação de resultado observado ou retry autorizado como nova ação vinculada. Registrar ator, evidência e decisão.
- Aprovação não é guardada como segredo, mas seus argumentos podem conter dados sensíveis e precisam de redação nas superfícies de export/log.

### 12.3 Sem TTY e exit codes

Contrato inicial a fixar em F0 e manter estável:

| Código | Resultado |
| --- | --- |
| 0 | Execução/comando concluído com sucesso |
| 1 | Falha de execução/provider/ferramenta |
| 2 | Argumento/configuração inválida ou requisito de configuração ausente |
| 3 | Aprovação pendente persistida; requer retomada explícita |
| 4 | Operação in-doubt exige reconciliação |
| 130 | Execução encerrada por cancelamento/interrupção em modo avulso |

No chat, Ctrl+C ativo cancela o run e volta ao editor sem encerrar o processo. Sem TTY, nunca abrir Huh, esperar tecla ou solicitar aprovação interativa. Emitir IDs de sessão/checkpoint/operação e instrução de retomada. stdout contém somente texto escolhido ou eventos JSONL válidos; stderr recebe diagnósticos compatíveis com o formato. Broken pipe cancela a execução e fecha recursos.

### 12.4 Encerramento

Parar entrada → cancelar árvore → interromper filas e processos → aguardar trabalhos dentro do prazo → salvar estado consistente/identificar operações incertas → liberar views/inscrições → fechar componentes em ordem inversa → fechar stores → restaurar terminal. Usar contexto de cleanup separado, com timeout, depois de cancelar o contexto da execução. Falha abrupta deve ser recuperável pelo estado já durável; shutdown gracioso não é garantia contra SIGKILL ou queda de energia.

## 13. Estratégia de validação e CI

### 13.1 Camadas de testes

| Camada | Cobertura necessária | Introdução |
| --- | --- | --- |
| Unidade determinística | Config/proveniência, políticas, paths, envelopes, estado da UI, redaction, accounting e comandos | F0 em diante |
| Contratos SDK | Consumidor externo, APIs públicas, factories, scopes, OAuth e stores | F1 |
| Integração de execução | Approval/deny, checkpoint/resume, RuntimeView/OperationStore, cancelamento e paridade de modos | F1 |
| Terminal/PTY | Streaming longo, scrollback único, resize, Unicode, paste, teclado, erro e restauração | F1/F2 |
| Headless | stdin pipe, sem TTY, texto/JSONL, exit codes, approval pendente, broken pipe | F1 |
| Concorrência | Leituras paralelas, escritas exclusivas, cancelamento na fila, limites/profundidade e múltiplos CLIs | F3 |
| Recovery com falhas injetadas | Antes/depois do efeito e antes/depois do commit, migrations, in-doubt e reconciliação | F1/F3 |
| Sessões/tarefas | Branch, compaction, retry, steering, follow-ups e snapshot hierárquico | F3 |
| Extensões | MCP stdio/HTTP, memória, WASM, gerações e reconfiguração | F4 |
| Smoke real opt-in | Provider/modelo real, OAuth quando aplicável e prova vertical | F1/F5 |
| Pacotes | Build CGO=0, artefatos extraídos e execução em runner nativo | F5 |

Usar providers fake, relógios e IDs controláveis e HTTP fixtures sanitizadas. Credenciais reais ficam exclusivamente em testes opt-in; CI padrão não depende de conta, cobrança ou rede de provider. Não registrar tokens em fixtures. Testes devem verificar efeitos e invariantes observáveis, não apenas reproduzir funções internas.

### 13.2 Matriz de CI

- Linux, macOS e Windows com o patch estável de Go 1.27.x fixado na F0 (inicialmente 1.27.4), igual ao desenvolvimento; `gofmt`, lint fixado, `go vet`, testes e build do produto sem cgo.
- Validar feature tags que efetivamente habilitam LSP, MCP, memória, SQLite e WASM no Opera, incluindo diagnóstico de builds sem capacidade quando mantidos.
- Detector de corridas em jobs separados onde suportado: admitir cgo e compilador somente nesses jobs, pois `-race` pode exigir cgo; isso não altera a obrigação de builds de distribuição com `CGO_ENABLED=0`.
- PTY/terminal nos sistemas suportados; Windows precisa validar o caminho real de console/pseudoconsole, não apenas golden strings de Unix.
- Checks de arquitetura impedem imports privados do Y e dependências UI → provider/tool runner. Verificar ausência de `replace` local e caminhos absolutos na distribuição.
- `go mod verify`, compilação limpa, análise de dependências/licenças e geração de checksums nos gates de pacote. Evitar depender de ferramentas globais não fixadas.
- Checks do Y acompanham as alterações no SDK; checks do Opera não substituem testes de contrato no Y.

## 14. Cenário final de aceite

Executar em repositório fixture com resultado verificável e provider fake roteirizado; repetir a prova principal com provider real opt-in:

1. Configurar provider/modelo e `ask`; abrir chat e carregar AGENTS.md/arquivos.
2. Pedir uma mudança de coding com teste; criar plano e tarefas com IDs persistentes.
3. Delegar leitura a subagentes dentro dos limites e observar seus eventos na mesma sessão.
4. Solicitar duas mutações e comprovar serialização no workspace; aprovar uma e negar outra sem efeito.
5. Executar ferramenta de shell/teste pelo mesmo controle de operações; observar diff e resultado.
6. Enviar follow-up e steering; interromper preservando o último estado consistente e fechar processos.
7. Retomar por ID; verificar hierarquia, política, histórico, uso e resultados sem replay de mutações concluídas.
8. Injetar efeito incerto; comprovar bloqueio de retry até reconciliação explícita.
9. Criar branch, compactar e exportar sessão com proveniência e sem segredos.
10. Exercitar memória, MCP e WASM fixtures e inspecionar ferramentas, schemas e gerações.
11. Repetir a tarefa nos modos `run` e `worker` pelo mesmo serviço; em headless, persistir aprovação pendente e retornar código 3/JSONL válido.
12. Executar o artefato empacotado em Linux/macOS/Windows e confirmar terminal restaurado, nenhum filho sobrevivente e ausência de backend `y`.

## 15. Riscos e decisões que precisam ser fechadas

| Risco | Tratamento / ponto de decisão |
| --- | --- |
| APIs públicas incompletas ou divergência local/tag Y | Inventário F0, consumidor externo e publicação do SDK como gate F1 |
| Snapshot contorna wrapper durável | Teste obrigatório no Y com RuntimeView real e store instrumentado antes da UI ampla |
| Scrollback duplicado/flicker em saída longa | Prova mínima F1, protocolo de commit e testes PTY em F2 |
| Persistência inconsistente entre tarefas/efeitos/checkpoint | Definir transações e recuperação por estados; falhas injetadas em cada fronteira |
| Diff aprovado muda antes da execução | Revalidar sob exclusão e renovar aprovação quando necessário |
| Shell ou extensão cria trabalho destacado | Ownership de processos, supervisão de árvore e testes por plataforma; diagnosticar limites de controle do SO |
| Escrita escapa por MCP/WASM/LSP ou segundo CLI | Enforcement central, classificação conservadora e lock/coordenação interprocesso |
| SQLite, tags ou bibliotecas incompatíveis com CGO=0 | Compilação conjunta F0 e build de cada capacidade nas três plataformas |
| Custos/modalidades variam por provider | Capabilities explícitas, fixtures por adapter e distinção entre conhecido/estimado/indisponível |
| Reconfiguração invalida operação pendente | Safe points, geração pinada e validação na retomada; nunca trocar handler silenciosamente |
| Recursos públicos descobertos tardiamente | Matriz viva revisada em F0, F4 e F5; gap aplicável bloqueia fechamento |

## 16. Checklist de conclusão da entrega

- [ ] F0–F5 concluídas com evidências e decisões pendentes resolvidas.
- [x] Módulo/binário corretos, Go/Charm fixados e SDK Y publicado sem dependência local.
- [x] Todos os comandos e slash commands do escopo implementados e documentados.
- [x] Chat/run/worker compartilham backend, política, fila e persistência.
- [x] OAuth, adapters, task restore e controle ativo usam APIs públicas do Y.
- [x] RuntimeView não contorna OperationStore, inclusive com extensões e reconfiguração.
- [x] Sessões, branches, compaction, retry, steering, follow-ups e subagentes têm integração passando.
- [x] Cancelamento e crash recovery não repetem efeitos incertos; reconciliação é explícita.
- [x] Configuração/segredos, headless, acessibilidade e diagnóstico cumprem os contratos.
- [x] Matriz final cobre todas as capacidades públicas aplicáveis da versão efetivamente usada.
- [ ] CI e cenário final passam; pacotes multiplataforma estão prontos localmente/como artefatos CI.
- [x] Nenhum daemon, execução remota, worktree automática ou publicação do Opera foi incluído.

O roadmap se encerra com software implementado e empacotável; scaffolding, mocks ou uma demonstração parcial não satisfazem o aceite final.
