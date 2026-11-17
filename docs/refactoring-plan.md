# Plano de refatoração do Opera — Corvis/Y e TUI por composição

Data: 2026-11-16. Este documento define uma migração executável; a conclusão de cada
etapa deve ser registrada em `docs/refactoring-validation.md` com commit e evidência.

## Fundamento e fontes

Opera é um produto composto sobre o SDK público Y. O modelo chamado Corvis neste
projeto segue o core controlado e composição aberta descritos em
`y/docs/philosophy.md`, `runtime-composition.md`, `component-contract.md`,
`effects-and-durable-state.md` e `runtime-protocol.md` no repositório irmão Y.
As referências locais explicam a direção; somente APIs presentes na versão
publicada em `go.mod` podem ser usadas. Não se presume paridade entre o checkout
local e Y v1.3.4.

A unidade extensível é uma capacidade com granularidade útil de substituição:
provider, ferramenta/tool pack, contexto, persistência, política, integração ou
contribuição de apresentação. Funções auxiliares e invariantes não são plugins.
O Y possui o loop, execução e protocolo do agente; Opera possui casos de uso,
configuração, defaults e composição do produto.

## Diagnóstico do estado inicial

- `internal/harness/open.go` já compõe `runtime.Harness`, mas mistura abertura de
  stores, providers, ferramentas, hooks, runners e serviço em uma função extensa.
  Dependências como Work e Extensions atravessam closures e ordem implícita.
- `internal/harness/providers.go` importa implementações concretas e seleciona
  providers por switch. A enumeração também aparece na configuração e CLI.
- `app.Service` depende de `*sqlite.Store`, `*persistence.Store`, `*skills.Manager`
  e chama `contextfiles.Load` diretamente. `Work` também recebe store concreto.
- `internal/extensions` já adapta o Host do Y para MCP, memória e WASM; deve ser
  aproveitado e não substituído por outro runtime de plugins.
- `internal/ui/chat.go` concentra editor, seleção, formulários, navegação,
  conversação e atividade. Existem portas de apresentação úteis, como Executor.
- `make check` falha inicialmente: import de `os/exec` em
  `internal/ui/attachments.go` viola a fronteira de UI já verificada pelo projeto.
- Há alterações locais prévias em AGENTS, DESIGN, app/user_question e arquivos de
  UI, além de teste de serialização não rastreado. Preservá-las e não incorporá-las
  silenciosamente aos commits da migração. Snapshot inicial: `/tmp/opera-pre-refactor.patch`.

## Arquitetura alvo

```text
cmd/opera -> cli (bootstrap/adaptador de entrada) -> app (casos de uso e portas)
                  |                               ^
                  +-> ui (ações/eventos) -----------+
                  +-> harness (composition root) --+-> adapters / components
                                                       |
                                                  Y público
```

`cli` é hoje também o bootstrap: pode chamar o harness para abrir um runtime,
mas não transfere regras de execução para a entrega. `app` não importa harness,
CLI, UI, drivers SQL ou providers concretos. Portas são definidas pelo consumidor,
usando contratos públicos do Y quando estes já resolvem o problema. Tipos de
registros podem ser compartilhados em um pacote sem I/O; interfaces com uma
única implementação ainda são úteis quando delimitam efeitos concretos.

Providers concretos e metadados ficam em adapters/componentes nativos. Um catálogo
explícito e imutável por instância recebe registros; não há registry global,
registro por `init` nem fallback silencioso. Adicionar um provider exige seu
adapter e registro na composição padrão, sem switch no core, UI ou validação
estática por nome. Catálogo, configuração e seletor devem concordar. Autenticação
específica permanece no adapter/fluxo de login, nunca no loop do agente.

`internal/extensions` continua gerindo integrações e configuração do Opera sobre
os contratos do Y. O catálogo de factories não substitui `component.Runtime`,
`component.Manager` ou `extensions.Host`: lifecycle e gerações pertencem ao Y.
Persistência padrão pode continuar SQLite, mas é injetada por portas. Ferramentas
nativas são contribuições selecionadas explicitamente na composição.

## Etapas e critérios de aceite

### 0. Contratos e decisões

Escrever este plano, ADR de portas/composição, paradigma de TUI e AGENTS completo.
Registrar baseline e preservar alterações locais. Commit apenas dos documentos
autorizados; revisão do plano antes da implementação.

### 1. Providers como capacidades registradas

Extrair factories, descritores e catálogo para uma fronteira de adapters própria.
Manter APIs de compatibilidade só quando houver consumidor real; catálogo usado
na execução e descoberta deve ser o mesmo. Preservar lookup explícito de credenciais,
base_url e semântica de assinatura. Remover listas fechadas de providers das
fronteiras genéricas; validar disponibilidade na composição.

Aceite: provider fake/customizado registrado funciona sem alterar seleção central;
IDs duplicados/desconhecidos falham claramente; catálogos não expõem mutação global;
nenhum provider concreto é importado no app ou harness genérico. Tests de regressão
de providers existentes continuam passando.

### 2. Portas da aplicação e efeitos

Introduzir contratos pequenos para runner, checkpoints/eventos/operações/transcript,
registros, skills e carregamento de contexto onde consumidos. Evitar uma interface
universal de infraestrutura e evitar copiar tipos do SDK. Migrar Service e Work;
abrir e fechar concretos somente no owner da composição. Extrair efeitos de clipboard
/processo da TUI para adaptador injetado através da entrega. Revisar leitura de
arquivos para aprovação: a coleta do preview é um efeito por porta; a comparação,
política e decisão continuam em app.

Aceite: app não importa drivers SQLite nem adapters concretos; testes usam doubles
pelas portas sem construir providers reais; UI não executa processos diretamente;
continuação, aprovação, imagens e recuperação mantêm contratos e formatos duráveis.

### 3. Composição por componentes coesos

Dividir Open por responsabilidades (storage, provider, tools, runner, application,
inspection) e centralizar keys compartilhadas. Expressar dependências de cada
componente nos descritores; substituir comunicação lateral relevante por serviços
declarados. Permitir seleção/injeção de componentes/tool packs sem alterar o fluxo
central. Configuração padrão continua funcional sem MCP, WASM ou memória opcional.
Factories sem efeitos; Start adquire e registra cleanup imediatamente.

Aceite: Harness público permanece dono do grafo; rollback e fechamento inverso
são testados, inclusive falha parcial; ferramentas anunciadas e handlers mantêm a
mesma geração do Host; nenhuma mudança de schema ou identidade durável gratuita.
Nenhum recurso inicia fora de owner conhecido.

### 4. TUI por composição inspirada em SwiftUI

Aplicar `docs/tui-development.md`, DESIGN e a skill local. Extrair submodelos por
responsabilidade real (composer, seleção/painel, atividade e conversação), começando
por fluxos existentes. O shell Bubble Tea coordena mensagens e efeitos; componentes
recebem estado/dados e devolvem intenção. Não criar DSL SwiftUI nem framework de
renderização sobre Charm. Evitar mover campos sem transferir sua responsabilidade.

Aceite: View determinística, sem I/O/mutação; Update possui estado; efeitos retornam
mensagens, com geração para descartar resultados obsoletos; teclado, foco, resize,
Unicode, NO_COLOR vazio, no-motion, acessibilidade e scrollback preservados. Extrair
componentes testáveis e realmente usados, mantendo identidade visual existente.

### 5. Regras automatizadas e fechamento

Ampliar checks de arquitetura para impedir dependências inversas, providers concretos
no core e efeitos proibidos em UI. Testar o próprio verificador com fixtures válidas
e inválidas. Atualizar documentação de extensões e exemplo executável que demonstre
substituição sem modificar core. Revisar o diff integrado e corrigir achados.

Aceite final: `make check`, `make lint`, `make matrix`, `make inventory` sem drift,
`go mod tidy` sem drift e testes race no host com CGO habilitado. CLI smoke offline
(help/version/doctor), regressões TUI e SDK consumer. Não alegar execução nativa em
outros sistemas a partir de cross-compilation. Testes live só com autorização e
credenciais existentes; indicar claramente skips e limites.

## Estratégia de commits e compatibilidade

Cada etapa recebe commit coeso após testes relevantes. Não commitar segredos,
binários, caches, snapshots temporários ou mudanças prévias fora do escopo. Quando
um arquivo já estiver modificado, stage apenas hunks novos e mantenha o baseline
local no working tree. Não usar reset/checkout destrutivo ou stash das mudanças do
usuário. Registrar revisão, comandos, resultados e limitações no relatório final.
Sem alterar UX, comandos, formatos JSONL, persistência ou defaults incidentalmente.
Se uma capacidade faltar no Y público, registrar o bloqueio; alteração no SDK deve
ser publicada antes de integrar, sem replace/go.work ou imports privados.
