# Desenvolvimento do Opera

## Objetivo e fontes de verdade

Opera é um produto de agentes de código composto sobre o SDK público Y, seguindo
seu modelo Corvis: core controlado, composição aberta e capacidades substituíveis.
Leia antes de alterar arquitetura:

- `docs/refactoring-plan.md`: arquitetura alvo, migração e critérios de aceite.
- `docs/adr/`: decisões duradouras e seus limites.
- `DESIGN.md`: identidade visual atual, terminal e acessibilidade.
- `docs/tui-development.md` e `.agents/skills/opera-tui-ui/SKILL.md`: paradigma TUI.
- `docs/cli-contract.md`, `docs/configuration.md`, `docs/extensions.md` e
  `docs/ui-extensions.md`: contratos de uso e integração.
- No repositório irmão Y, `docs/philosophy.md`, `docs/component-contract.md`,
  `docs/runtime-composition.md`, `docs/effects-and-durable-state.md` e
  `docs/runtime-protocol.md`: fundamento do SDK. Não exigir esse caminho local em
  build/testes nem presumir APIs não publicadas a partir desses documentos.

Use `rtk` como prefixo dos comandos shell conforme `/Users/yuri/.codex/RTK.md`
quando esse arquivo/ferramenta estiver disponível; `rtk proxy` executa comandos
sem filtragem. Use `rg` para navegação. Em repositórios com `.codegraph/`, consulte
CodeGraph antes de procurar/ler código; não crie índice sem solicitação.

## Core e direção das dependências

O Y possui loop do agente, turns, ferramentas, estado/snapshots, protocolo de eventos,
retomada, idempotência, runtime views e lifecycle de componentes. Não copie nem
mantenha outro loop, registry de execução ou gerenciador de gerações no Opera.
Opera possui configuração/defaults, casos de uso, política do produto, composição,
adapters e apresentação. Não invoque o binário Y como backend.

Use arquitetura de portas e adaptadores pragmática:

- `cmd/opera`: entrada mínima e saída do processo.
- `internal/cli`: adaptador de entrega e bootstrap; traduz flags/entrada/saída,
  abre a composição e conecta UI. Não define semântica de execução.
- `internal/ui`: apresentação, estado de interação e tradução de intenções em
  ações. Não conhece providers, drivers, harness ou execução de ferramentas.
- `internal/app`: casos de uso, ações/eventos, contratos/portas consumidoras e
  regras do produto compartilhadas por chat, CLI/headless e workers.
- `internal/harness`: composition root, factories/componentes, bindings,
  configuração e ownership dos recursos sobre `runtime.Harness` público.
- Adapters de providers, persistência, contexto, credenciais, skills e plataforma:
  implementam portas; detalhes concretos não entram em app.
- `internal/extensions`: configuração, consentimento e gestão das integrações do
  Opera por `extensions.Host`/componentes públicos do Y.

Dependências apontam para contratos do consumidor e SDK público. App não importa
CLI/UI/harness, providers concretos, driver SQL ou implementação SQLite. UI não
importa agent/providers/tools, persistence ou os/exec. Não crie dependência entre
extensions; use serviços/contratos compartilhados. CLI pode compor adapters no
bootstrap; regras comuns continuam em app. Testes externos de integração podem
compor concretos para verificar a fronteira; o core de produção não pode.

Portas são pequenas e coesas, definidas onde são consumidas. Reutilize interfaces
públicas Y em vez de espelhá-las. Não envolva cada método do SDK em wrapper sem
variação real. Tipos de dados compartilhados devem ser independentes de I/O.
SQLite pode ser o default; não pode ser requisito oculto dos casos de uso.

## Componentes, providers e extensibilidade

Toda capacidade selecionável, substituível ou removível tem fronteira explícita:
providers, ferramentas/tool packs, contexto, persistência, política, transporte e
contribuições de UI. Helpers internos e invariantes não precisam virar plugins.
Componentes compilados e integrações opcionais seguem os mesmos princípios de
contrato e lifecycle, mesmo com diferentes níveis de confiança.

Um provider novo implementa contrato público Y, declara metadados/factory e entra
no catálogo da composição. A execução, configuração e descoberta usam o mesmo
catálogo. Não adicionar switches ou listas de providers no core/UI. Não usar
registro por init, estado global mutável, fallback silencioso ou detecção de
credencial que escape ao resolver configurado. Detalhes de OAuth/assinatura e
transporte ficam no adapter apropriado; não se tornam regra do agente.

Use `component.Definition`, `Descriptor`, `Key[T]` e serviços do Y para composição.
Declare ID, versão do componente, serviços fornecidos/requeridos, bindings,
versões de contrato e capabilities pertinentes. Versão de implementação não é
versão de serviço. Keys compartilhadas têm um único proprietário.
Um catálogo de factories seleciona componentes; ele não substitui o runtime Y.

Factories não abrem arquivos/conexões nem iniciam workers. Start adquire recursos
e registra ownership imediatamente com Scope.Track/Scope.Go ou API pública
equivalente. Declare dependências no grafo: não dependa da ordem de slices ou
variáveis capturadas para comunicar serviços entre componentes. Valores mutáveis
compartilhados precisam de owner e sincronização explícitos.

O caminho padrão funciona sem MCP, WASM ou memória opcionais. Ausência de capacidade
obrigatória falha antes da ativação com erro contextual. Falha parcial desfaz
registros e aquisições; não deixa goroutines, processos ou handlers órfãos.
Fechamento é idempotente, em ordem inversa de dependência, tenta todos os closers e
agrega erros. Interrompa novas ações, cancele trabalho, drene e feche stores por
último. Use contexto de cleanup independente e espera limitada, preservando a
semântica do Close do Y e deixando claro quando o cleanup continua em background.

Mudanças dinâmicas usam safe points/runtime views do Y. Ferramentas anunciadas,
handlers, hooks e contexto de uma operação pertencem à mesma geração. Não troque
registrations diretamente durante turn em andamento. Preserve a checagem de
identidade e aprovação ao retomar. Não prometa hot reload para componentes estáticos
sem suporte real e teste no runtime publicado.

## Efeitos, segurança e dados duráveis

Passe context.Context, cancelamento, limites e erros com contexto. Evite goroutines
soltas, canais sem owner, loops periódicos ociosos e contexto Background para
esconder cancelamento. Teardown e commits duráveis podem precisar de contexto
independente; documente propósito e prazo.

Dependência disponível não concede autorização. Capabilities, política,
consentimento, limite de workspace e aprovação são verificados nas fronteiras de
efeito. Native plugin é código confiável no processo; interface não é sandbox.
Código não confiável exige WASM/processo isolado e acesso mediado ao host. Preserve
consentimentos por digest e invalide-os quando configuração/conteúdo mudar.

Classifique aquisições temporárias, dados duráveis e emissões externas. Cleanup
fecha conexões/processos/registros; não apaga transcript, checkpoints, memória ou
arquivos do usuário, nem desfaz envio externo. Compensação só existe quando
explicitamente implementada; não substitui aprovação, auditoria e idempotência.

Persistência obrigatória falha na inicialização se indisponível. Modo degradado
precisa ser explícito e observável; erros de escrita continuam sendo erros.
Não faça fail-open silencioso para gravações. Preserve operações in_doubt,
recuperação sem replay de efeitos, limites de concorrência e identidade de sessão.
Não altere schema, JSONL, IDs, comandos ou HarnessVersion sem necessidade concreta,
compatibilidade/migração e teste de recuperação.

Nunca exponha tokens, segredos ou conteúdo sensível em logs, snapshots e commits.
Credenciais passam por portas/resolvers; não conceda acesso implícito a ambiente,
filesystem, rede, shell ou segredos a uma extension. Valide/sanitize dados externos
nas fronteiras, incluindo texto/ANSI e contribuições de UI.

## Paradigma TUI inspirado em SwiftUI

Para qualquer mudança em `internal/ui`, aplique a skill local
[opera-tui-ui](.agents/skills/opera-tui-ui/SKILL.md) e leia DESIGN.md. A inspiração
SwiftUI orienta composição, identidade estável e estado explícito; Go/Charm é a
implementação. Não crie DSL de Views, bindings por ponteiros mutáveis ou framework
paralelo. Veja o mapeamento detalhado em `docs/tui-development.md`.

- Tela é composição: shell coordena foco/navegação e encaminha mensagens;
  submodelos possuem estado de interações coesas. Funções puras bastam para peças
  sem estado. Não extraia structs que só duplicam os campos do shell.
- Uma fonte de verdade: cada estado possui owner único. Componentes filhos recebem
  valores/dependências necessários e emitem intenções tipadas; não alteram outros
  componentes por comunicação lateral.
- Update faz transições. Init/Update emitem tea.Cmd para efeitos; resultados voltam
  como tea.Msg. Identifique run/item/geração e descarte respostas obsoletas.
- View é determinística a partir de estado, dimensões e tema; sem I/O, goroutines,
  alteração de estado, ticks ou execução de comandos. Não consulte provider/store.
- Use Bubble Tea para ciclo de mensagens; Bubbles para textarea/list/viewport,
  keymaps e spinner; Huh para formulários/seletores/prompts; Lip Gloss para layout
  e estilo; Glamour para Markdown; Harmonica para movimento útil durante atividade.
  Antes de implementar controle próprio, verifique o componente Charm existente.
  Registre limitação técnica concreta para qualquer substituição.
- Tema é dado semântico central em theme.go: não espalhe cores literais. Terminal
  fornece fonte, background e foreground. DESIGN prevalece sobre referência móvel.
- Layout usa largura de colunas Unicode/ANSI, clipping/wrap e dimensões atuais.
  Renderize o conteúdo visível; reutilize renderizadores caros com invalidação
  explícita por conteúdo/largura/tema. Não otimize sem custo conhecido.
- Preserve chat em scrollback nativo, coluna única e sem alternate screen.
  Temporários devolvem foco ao composer. Teclado é o caminho principal.
- Respeite NO_COLOR por presença, inclusive vazio, --no-motion, resize e modo
  acessível. Estado é compreensível por texto sem cor ou movimento. Não anime idle
  nem mantenha cursor piscando. Caminho acessível é append-only sem raw-mode.
- Extensions fornecem dados de UI limitados e sanitizados; apresentação não recebe
  acesso livre ao runtime por callback de renderização.

Não usar todas as bibliotecas artificialmente em cada componente. Aproveite as
capacidades relevantes sem wrappers decorativos e sem copiar padrões de toque,
cartões ou navegação móvel que prejudiquem o terminal.

## Fluxo de desenvolvimento

1. Inspecione git status e mudanças locais; preserve trabalho prévio. Leia contratos,
   testes e ADRs da área. Diagnostique o caminho atual antes de adicionar subsistema.
2. Defina owner, porta, efeitos, lifecycle, erros e critério observável da mudança.
   Prefira composição explícita, funções/tipos pequenos, responsabilidades coesas e
   dependências acíclicas. Aplique SOLID/DRY sem abstração antecipada.
3. Implemente uma etapa completa, preservando comportamento público. Adicione testes
   de contrato/regressão para mudança relevante, limites e falhas; não testes que
   apenas espelham código. Evite novos pacotes/camadas sem consumidor concreto.
4. Rode checks relevantes e depois os gates de aceitação. Revise diff e documentação;
   registre decisões duradouras em docs/adr e atualize índice.
5. Quando a tarefa autorizar commits, faça commits coesos após validação, stage apenas
   alterações da tarefa e informe limitações. Não commite mudanças prévias sem
   autorização, não use reset/checkout/stash para descartá-las e não force push.

## Validação e ferramentas

Use a toolchain/dependências fixadas em `.go-version`, `go.mod` e Makefile.
Build normal: GOWORK=off, CGO_ENABLED=0 e tags feature_lsp,feature_wasm_ext.

- `rtk proxy make check`: formato, arquitetura, módulos, vet, testes, consumidor SDK
  e build CGO-free. Não desative checks para acomodar violação de fronteira.
- `rtk proxy make lint`: linter fixado pelo projeto; consulte skill Go lint ao
  alterar configuração/supressões. Corrija causa, não adicione nolint genérico.
- `rtk proxy make matrix`: build de Linux/macOS/Windows amd64/arm64 e compilação dos
  testes de plataforma. Não equivale a execução nativa nessas plataformas.
- `rtk proxy make inventory`: inventário SDK reproduzível; não deve haver drift
  incidental. `go mod tidy` também não deve alterar módulos incidentalmente.
- Race: use CGO_ENABLED=1 com a toolchain/tags fixadas e `go test -race ./...`.
- CLI smoke offline: --help, --version e doctor --format jsonl, sem credencial/TTY.
- Para TUI, teste estado/transições, foco, cancelamento, geração obsoleta,
  resize, Unicode/ANSI, NO_COLOR vazio, no-motion, acessibilidade e PTY/ConPTY.
- Para componentes, teste grafo inválido, falha parcial, cleanup inverso/idempotente,
  cancelamento, ausência de opcionais e substituição por doubles/factories.

Não execute testes live pagos/externos sem autorização e credenciais disponíveis.
Registre skips e diferencie falha preexistente de regressão. Nunca anuncie check
como aprovado sem sua execução. Exceções de arquitetura exigem razão concreta e
ADR; não converta restrição de implementação em promessa de produto.

## Evolução do SDK Y

Quando capacidade necessária pertencer ao SDK e não puder ser atendida corretamente
no Opera, podemos alterar o repositório Y e abrir MR. Integração final usa API pública
e publicada. Não use imports privados, replace, go.work ou cópias locais como solução
permanente. Valide a versão realmente fixada antes de usar contrato novo, atualize
inventário/compatibilidade quando necessário e documente bloqueio se publicação
for requisito externo ainda não atendido.
