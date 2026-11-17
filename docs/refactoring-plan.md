# Refatoração do Opera: core Y, módulos e UI

Data da revisão: 2026-11-17.

Este plano substitui o arranjo genérico `app`/`harness` e a aplicação excessiva de
ports and adapters a código interno. A arquitetura do Opera usa o runtime público
Y como fundação acoplada ao core; módulos internos são coesos e podem colaborar
diretamente; adapters isolam integrações externas e efeitos substituíveis.

## Fundamento

Y é parte da arquitetura do produto, não um provider nem um adapter. O core usa
diretamente o `runtime.Harness` público, o modelo de componentes, o lifecycle, as
views e os protocolos publicados pelo SDK. Não manter um segundo loop, registry de
execução ou gerenciador de gerações no Opera. Não invocar o binário Y.

O core controla composição e políticas do produto. Os módulos de capacidade mantêm
regras e estado coesos. Adapters implementam integrações externas concretas, com
um submódulo próprio por provider. A UI traduz interação em operações do produto.
O limite de um pacote deve seguir ownership e direção das dependências, não uma
meta abstrata de desacoplamento.

## Estrutura alvo

```text
cmd/opera/
internal/
  core/
    runtime/                 # integração/lifecycle do Harness público Y
    composition/             # definitions, keys, bindings e grafo Y
    configuration/           # defaults e política de composição do produto
  modules/
    execution/               # execução, Work e estado que hoje é mutuamente ligado
    sessions/                # somente quando possuir fluxo e estado independentes
    approvals/               # política/decisão do produto, não o leitor de arquivos
    context/                 # dados e regras de contexto independentes de I/O
    skills/                  # catálogo/regras do produto para skills
    extensions/              # gestão do produto sobre APIs públicas de extensões Y
    workspace/               # identidade/regras de workspace
  adapters/
    providers/
      anthropic/
      google/
      openai/
        subscription/
      openai_compatible/
      openrouter/
      custom/
    persistence/sqlite/
    credentials/{file,keyring}/
    context/filesystem/
    skills/filesystem/
    workspace/{filesystem,locks}/
    platform/
  ui/
    cli/                      # comandos, flags, stdin/stdout e bootstrap de entrega
    tui/
      chat/
      composer/
      conversation/
      navigation/
      approvals/
      authentication/
      theme/
```

É um mapa de ownership, não uma obrigação de criar diretórios vazios. Criar ou
dividir pacote quando existir comportamento, dono e benefício de navegação claros.
O projeto continua com um único `go.mod`; os submódulos são packages Go, não módulos
Go independentes.

`core/composition` é o composition root: conhece o Y, os módulos e os adapters
concretos necessários para montar a configuração selecionada. `core/runtime`
controla o Harness Y e seu lifecycle. `cmd/opera` permanece mínimo. `ui/cli` chama
o core para iniciar e interagir com o produto. `ui/tui` apresenta dados e envia
intenções; não importa adapters, providers ou implementação do Harness.

## Regras de dependência

- `core` pode importar módulos e adapters para compor o produto. Módulos não
  importam `core` nem a composição.
- Módulos podem colaborar por chamadas diretas a funções/serviços concretos quando
  a dependência for coerente e acíclica. Não criar interface só para evitar uma
  dependência interna legítima.
- Dividir um módulo apenas quando cada lado tiver dono, estado e direção de
  dependência explícitos. Se `A` e `B` precisarem chamar-se reciprocamente,
  mantê-los juntos até existir um corte real que elimine a reciprocidade.
- Módulos podem usar contratos públicos do Y que façam parte de sua lógica. O
  lifecycle `runtime.Harness`, a montagem do grafo e o ownership de componentes
  pertencem ao core; não abstraí-los como “provider Y”.
- Providers, drivers SQL, OAuth/transporte, keyring, filesystem, clipboard e
  processos ficam em `adapters/` no submódulo que os possui. Cada provider reúne
  seu cliente, autenticação, metadados e factory. A composição agrega factories;
  os adapters não importam o composition root.
- A UI depende de operações/dados de produto. Providers não conhecem UI e a UI
  não escolhe providers por switch ou por nome concreto.
- Portas pequenas são reservadas a limites de efeito/substituição reais, como
  persistência, credenciais, leitura de contexto ou transporte. Não replicar
  interfaces do SDK Y nem criar uma camada `domain/application/infrastructure`
  para cada capability.
- Catálogos são instâncias explícitas sem registro por `init`, estado global
  mutável, fallback silencioso ou segunda autoridade de lifecycle.

## Padrões de projeto

| Padrão | Uso no Opera |
|---|---|
| Composition root / Builder | Core compõe o grafo Y e monta somente capabilities selecionadas. |
| Adapter | Fronteira para API externa, banco, credencial, filesystem ou processo. |
| Strategy | Provider ou política realmente intercambiável sob o mesmo contrato. |
| Factory / catálogo explícito | Criar e descobrir instâncias por configuração, sem switches distribuídos. |
| State machine | Lifecycle, aprovação, retomada e fluxos TUI com transições observáveis. |
| Command / message | Bubble Tea: efeitos são comandos; resultados retornam como mensagens identificadas. |
| Repository | Só quando encapsula semântica durável de leitura/escrita, não como wrapper de SQL. |
| Decorator | Cross-cutting concreto como accounting/telemetria sem alterar o provider base. |

Composição simples tem preferência sobre padrões decorativos. SOLID, coesão,
responsabilidade única, dependências explícitas, erros com contexto, ausência de
estado global e cancelamento com ownership são critérios de revisão, não objetivos
para multiplicar packages e interfaces. DRY vale quando uma abstração deixa o
comportamento mais claro; duplicação pequena e local pode ser mais simples.

## TUI

Aplicar a filosofia documentada em `docs/tui-development.md` e na skill
`.agents/skills/opera-tui-ui/SKILL.md`: composição declarativa idiomática em Go,
estado com dono único, identidade estável e `View` determinística. O shell coordena
foco/mensagens; submodelos possuem interações coesas. `Update` aplica transições e
emite comandos; efeitos voltam como mensagens e respostas obsoletas são descartadas.
Não criar DSL, bindings mutáveis nem framework UI paralelo. DESIGN.md determina a
identidade visual terminal.

## Sequência de migração

1. Atualizar decisões e `AGENTS.md` para registrar core Y, módulos colaborativos,
adapters externos e `ui/cli` + `ui/tui`.
2. Fechar a migração dos providers em `internal/adapters/providers/<provider>`;
   resolver imports antigos, autenticação específica e catálogo padrão. Catálogo,
   login, descoberta e execução devem usar a mesma composição explícita.
3. Mover a composição atual de `internal/harness` para `internal/core`, preservando
   chaves, grafo declarado, startup, rollback, fechamento inverso e runtime Y
   público. Separar arquivos por builder apenas quando ownership ficar claro.
4. Migrar `internal/app` para `internal/modules/execution` junto com `Service` e
   `Work`, que colaboram diretamente. Atualizar consumidores na mesma etapa. Extrair
   sessions, approvals, context ou outras capabilities apenas quando o corte ficar
   acíclico sem interfaces artificiais.
5. Agrupar adapters externos em submódulos coerentes e colocar regras/catálogos
   internos no módulo que os possui. Remover caminhos e agregadores antigos quando
   não houver mais consumidores.
6. Mover CLI e TUI para `internal/ui/cli` e `internal/ui/tui`; manter CLI como
   fronteira de processo e TUI como apresentação. Preservar comandos e contrato.
7. Ampliar os checks arquiteturais para bloquear imports de providers fora de
   adapters/composição, imports de core por módulos, ciclos, dependências de UI em
   infraestrutura e restos dos pacotes antigos.
8. Revisar o grafo completo, documentação e diffs; validar os gates abaixo e
   registrar evidências em `docs/refactoring-validation.md`.

Cada etapa é completa e compilável antes da próxima. Mudanças estruturais devem
preservar flags, UX, formatos JSON/JSONL, esquema SQLite, IDs, retomada, erros,
autorização e lifecycle. Não usar replace/go.work/import privado do SDK Y como
solução. Se contrato público necessário estiver ausente, registrar o bloqueio e
parar essa integração até publicação compatível.

## Critérios de validação

- `go list ./...` resolve todo o grafo e as dependências têm direção acíclica.
- `make check`, `make lint`, `make matrix`, `make inventory`, `go mod tidy -diff`,
  testes com race e build CGO-free passam sem drift incidental.
- CLI smoke offline: `--help`, `--version`, `doctor --format jsonl`.
- Testes do core cobrem grafo inválido, falha parcial, cancelamento e cleanup
  idempotente em ordem inversa; testes de módulos cobrem transições e persistência
  durável; testes de adapters cobrem contrato/substituição.
- TUI preserva foco, cancelamento, descarte de geração obsoleta, resize, Unicode/
  ANSI, NO_COLOR vazio, no-motion, acessibilidade e scrollback.
- `make matrix` demonstra cross-compilation Linux/macOS/Windows amd64/arm64, não
  execução nativa nesses sistemas. Testes live pagos/externos ficam fora sem
  autorização e credenciais.

## Commits e trabalho preexistente

Registrar cada etapa com commit coeso após validação relevante, stageando somente
os hunks da migração. Alterações prévias do usuário em DESIGN, acessibilidade, UI
ou outros arquivos continuam no working tree e não entram nesses commits. Não usar
reset, checkout destrutivo ou stash para descartá-las; não fazer push forçado.
