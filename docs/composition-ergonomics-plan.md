# Plano: ergonomia de composição modular

Status: proposta de evolução arquitetural; não altera os ADRs aceitos nem
autoriza, por si só, execução de módulos externos.

## Objetivo

Fazer com que uma capacidade do Opera possa ser descoberta, selecionada e
combinada como uma peça de composição com contrato e lifecycle claros. Um
contribuidor deve conseguir adicionar uma implementação sem espalhar switches
por CLI, TUI, configuração e execução. Quem monta um perfil deve conseguir
entender quais capacidades entram nele e quais serviços cada uma fornece.

O alvo é a ergonomia de composição do DeepSeek Harness, adaptada ao Opera: o Y
continua sendo o runtime e o composition root continua sendo `internal/core`.
“Tudo é módulo” significa que toda capacidade selecionável tem uma definição de
composição; não significa que cada package, helper ou regra interna seja plugin.

## Estado atual

- `internal/modules/<capacidade>` organiza regras e estado por ownership; esses
  packages não correspondem automaticamente a componentes carregáveis.
- `internal/core.Open` constrói uma lista fixa de definições (`storage`,
  `provider`, `tools`, `runner`, `application`), com uma variação para inspeção.
- `component.Definition` e o `runtime.Harness` do Y já oferecem composição,
  dependências declaradas e lifecycle.
- Providers têm catálogo explícito de factories; tool packs e extensões também
  têm mecanismos próprios. A ergonomia ainda varia por capacidade e a seleção
  não tem um único modelo visível de perfil.
- Extensões WASM/MCP têm fronteiras de confiança diferentes de componentes Go
  compilados; não devem ser unificadas sob uma promessa genérica de plugin.

## Princípios e limites

1. Y continua dono do runtime, validação do grafo, geração, safe points e
   lifecycle. Opera não cria um segundo loop nem um segundo gerenciador de
   componentes.
2. `internal/core` monta a composição. Módulos de produto não importam o core.
3. Uma capacidade vira componente selecionável quando existe variação concreta
   (ativar, remover ou substituir). Helpers e invariantes permanecem internos.
4. Cada peça declara identidade, versão de implementação, serviços fornecidos e
   requeridos, bindings, capabilities e requisitos de configuração pertinentes.
   Versão do componente não substitui versão de contrato de serviço.
5. Factories descrevem e constroem definições; não abrem recursos externos nem
   iniciam workers. Aquisições pertencem ao Start/Scope e precisam de rollback.
6. Componentes compilados são código confiável no processo. Código de terceiros
   exige isolamento e acesso mediado; interface de componente não é sandbox.
7. A composição padrão funciona sem capacidades opcionais. Ausências obrigatórias
   falham antes da ativação, com erro contextual.
8. Preservar compatibilidade de CLI, configuração durável, sessões, aprovações,
   retomada, schema e contratos publicados do Y.

## Design proposto

Esta seção descreve formas e relações para guiar as etapas. Os nomes são
ilustrativos; antes de implementar, confirmar os campos e operações disponíveis
na versão publicada do SDK Y. `component.Definition`, `Descriptor`, `Key[T]`,
`ServiceRef`, `Scope` e `runtime.Harness` continuam sendo os contratos de
composição e lifecycle. Os tipos abaixo modelam seleção e ergonomia do Opera;
eles não repetem o grafo nem substituem o runtime Y.

### Mapa de relacionamentos

```mermaid
flowchart TD
    CLI[CLI / TUI bootstrap] -->|opções resolvidas| CORE[internal/core Composer]
    PROFILE[Runtime Profile] --> CORE
    CATALOG[Component Catalog] -->|factory por ID| CORE
    CONFIG[Resolved Config] --> CORE
    CORE -->|[]component.Definition| Y[runtime.Harness do Y]
    Y -->|Start por dependências declaradas| STORAGE[Storage component]
    Y --> PROVIDER[Provider component]
    Y --> TOOLS[Tools component]
    Y --> RUNNER[Runner component]
    Y --> APP[Application component]
    STORAGE -. fornece keys .-> PROVIDER
    STORAGE -. fornece keys .-> TOOLS
    PROVIDER -. fornece serviço Y .-> RUNNER
    TOOLS -. fornece registry Y .-> RUNNER
    RUNNER -. fornece runner .-> APP
    APP -. fornece execution.Service .-> CLI
```

As setas sólidas representam seleção e montagem; as tracejadas representam
dependências declaradas por `Provides`/`Requires` e resolvidas pelo Y. A ordem de
Start não deve ser codificada na ordem do slice. O core conhece factories
concretas para montá-las; cada componente obtém dependências com as keys do Y e
registra recursos no próprio scope.

Ponto de partida no código: `core.Open` monta as definições; `compositionBuilder`
e `componentDefinition` criam Definitions; `compositionKeys` é o owner das keys;
`ToolPack` é uma contribuição registrada pelo componente tools; e o catálogo de
providers já junta factory, metadados e métodos de autenticação. A primeira
refatoração deve encaixar essas formas no modelo acima, sem reescrevê-las todas.
Arquivos sugeridos, mantendo o package `core`: `composition_catalog.go`,
`composition_profile.go`, `composition_resolve.go` e `composition_summary.go`.

### Tipos de seleção no core

Manter esses tipos em `internal/core` enquanto compartilharem keys privadas e
detalhes do composition root. Não criar `internal/modules/composition` que dependa
do core, nem novo package só para dividir arquivos do builder.

```go
type ComponentID string
type ComponentSlot string // papel selecionável, ex.: "provider" ou "storage"

type ComponentFactory func(ComponentBuildInput) (component.Definition, error)

type ComponentRegistration struct {
    ID      ComponentID
    Slot    ComponentSlot // vazio para componente fixo, sem alternativa selecionável
    Build   ComponentFactory
}

type ComponentCatalog struct {
    entries map[ComponentID]ComponentRegistration
}

type ComponentSelection struct {
    Slot ComponentSlot
    ID   ComponentID
}

type RuntimeProfile struct {
    ID          string
    Components  []ComponentID
    Selections  []ComponentSelection
}

type ComponentBuildInput struct {
    Config  config.Resolved
    Options CompositionOptions
}

type CompositionPlan struct {
    ProfileID   string
    Definitions []component.Definition
    Summary     []SelectedComponent // IDs e metadados seguros para doctor
}
```

O trecho é um modelo, não código pronto. Ajustes recomendados antes de codificar:

- Catálogo deve ser imutável depois de construído; `NewComponentCatalog` valida
  IDs vazios/duplicados, slots e factories ausentes e retorna erro contextual.
- `Build` deve ser pura em relação a I/O: valida config e produz uma Definition.
  Abrir banco, provider, processo ou worker ocorre no Start e fica sob o scope.
- `ComponentID` identifica uma implementação; `ComponentSlot` identifica a
  capability selecionável. Só criar slot se houver mais de uma implementação ou
  opção real. Perfil lista componentes fixos e escolhe exatamente uma
  implementação por slot.
- Descriptor Y é a fonte de verdade para ID/versão/serviços do componente sempre
  que a API permitir inspeção. Não manter uma cópia de `Provides`/`Requires` no
  catálogo. Se o descriptor só estiver disponível após Build, comparar o ID
  declarado no registro com o descriptor e falhar se divergirem.
- `ComponentBuildInput` não contém instâncias de serviços requeridos. Essas vêm
  de `component.Require` no Start; isso deixa o grafo explícito e evita capturas
  laterais entre factories.
- `CompositionPlan` serve para validação e inspeção. O Harness Y continua dono da
  validação final, resolução, ordem, gerações e lifecycle. O plano não mantém um
  segundo resolvedor de dependências.
- `Summary` contém apenas perfil, IDs, slots, versão e requisitos não sensíveis;
  nunca inclui credenciais, valores secretos ou conteúdo de sessão.

### Interfaces e limites de cada peça

| Forma | Owner | Contrato / relação | Quando usar |
|---|---|---|---|
| `component.Definition` do Y | `internal/core` + factory do componente | Descriptor declara serviços fornecidos e requeridos; `Start` resolve `Key[T]` com `Require`, fornece com `Provide` e acompanha recursos no `Scope`. | Para capacidade com serviço consumido por outro componente ou lifecycle próprio. |
| `ComponentRegistration` / `ComponentCatalog` | `internal/core` | Mapeia ID para factory; `RuntimeProfile` seleciona ID por slot; não instancia recursos nem registra globalmente. | Para catálogo explícito de componentes compilados selecionáveis. |
| `RuntimeProfile` | `internal/core` / configuração de runtime | Lista componentes fixos e implementação escolhida por slot; perfil é resolvido uma vez antes de `NewHarness`. | Para conjuntos operacionais distintos, não para prompt/identidade do agente. |
| `Provider Catalog Entry` existente | `modules/providers/catalog` | Une factory, metadados, autenticação, search e capabilities do provider; adapter do provider implementa contrato público Y. | Para variação dentro do slot provider; não duplicar provider no catálogo genérico e no catálogo específico. |
| `ToolPack` / contribuição de tools | `internal/core` inicialmente | Contribui tools a um registry pertencente ao componente tools, com dependências explícitas e rollback de registro. | Para extensão de registry sem lifecycle independente; extrair como componente só se tiver serviços/recursos próprios. |
| `Execution Service` | `modules/execution` | Serviço de produto consumido pela UI; mantém execução, sessão, aprovação e retomada coesas até haver corte acíclico comprovado. | Não expor loop interno como plugin só para imitar o modelo externo. |
| `WASM/MCP extension` | `modules/extensions` + adapters/host | Host aplica grants, policy, consentimento, geração e APIs mediadas; código não recebe handles irrestritos. | Para código/integradores isolados conforme o contrato de trust existente. |

No desenho de tool packs, conservar a diferença entre **componente** e
**contribuição**: packs como `filesystem` e `git` adicionam ferramentas ao
registry que o componente tools possui. Um pack não precisa ganhar Definition
própria enquanto não fornecer serviço separado nem adquirir recurso/lifecycle
próprio. Se essa necessidade surgir, elevar o pack a componente e declarar suas
dependências via Y; não deixar o callback acessar serviços globais.

Interfaces Go devem nascer no consumidor que precisa substituir uma dependência e
representar uma fronteira real. Para factories e catálogos internos, preferir
tipos concretos e funções; não adicionar `Composer`, `Registry`, `Resolver` ou
`ModuleManager` interfaces por simetria. Não criar interfaces que copiem os
contratos Y.

### Fluxo de resolução e inicialização

1. CLI/TUI resolve configuração e constrói `CompositionOptions`; não seleciona
   implementações por switch de provider ou adapter concreto.
2. `core.ResolveComposition(profileID, catalog, config, options)` valida IDs,
   slots, defaults, exclusividade e requisitos antes de produzir factories.
3. Cada factory produz `component.Definition`; o core confirma identidade e
   chama `runtime.NewHarness(definitions...)`.
4. `Harness.Start(ctx)` controla dependências e inicialização. Componentes usam
   `Require`/`Provide` e registram imediatamente recursos com `Scope.Track` ou
   API pública equivalente.
5. A UI resolve o serviço de produto exposto pelo core; não importa component
   catalog, keys internas, provider concreto ou store.
6. Para inspeção, o core retorna/renderiza `CompositionPlan.Summary` ou usa uma
   runtime view pública do Y. Não consulta stores/providers na `View` da TUI.

Falha nos passos 2–3 não abre recursos. Falha no passo 4 delega rollback ao
lifecycle do Harness; recursos já abertos precisam estar registrados no scope no
mesmo passo em que foram adquiridos. A resolução e ativação acontecem uma vez por
runtime; mudança de geração segue o protocolo de safe point do Y.

### Padrões de projeto e uso correto

| Padrão | Aplicação proposta | Limite |
|---|---|---|
| Composition Root / Builder | `internal/core` transforma perfil + catálogo + config em Definitions do Y. | Builder só organiza montagem; não mantém lifecycle paralelo nem lista de execução global. |
| Explicit Factory + Catalog | Factories selecionadas por ID constroem provider/storage/componentes compilados. | Sem `init`, singleton mutável ou I/O na factory; catálogo é local à composição. |
| Strategy | Implementações distintas ocupam o mesmo slot e fornecem o mesmo contrato de serviço. | Usar apenas quando a variação é substituível; não criar Strategy para cada função interna. |
| Dependency Injection | `Descriptor` Y declara dependências; `Require`/`Provide` conecta serviços. | Não usar ordem de slice, closures compartilhadas ou service locator próprio. |
| Adapter | Provider, SQLite, keyring, filesystem, processo e rede traduzem efeitos externos. | Adapter fica no submódulo que possui protocolo/efeito e não escolhe composição. |
| Decorator | Accounting/telemetria envolve provider ou ferramenta sem mudar o contrato consumido. | Decorator deve preservar identidade, Close e erros do componente envolvido. |
| State Machine / Safe Point | Y gerencia lifecycle e gerações; execution mantém transições duráveis de run/session. | Não reconciliar registrations no meio de uma operação nem construir loop próprio. |
| Bundle/Profile | Perfil agrupa IDs explícitos e escolhas por slot. | Começar com perfis pequenos; sem linguagem geral de patches, dependências implícitas ou hot reload universal. |

### Exemplo de grafo alvo

```text
profile "default"
  storage: sqlite
  provider: provider.openai     # escolha pelo catálogo de providers existente
  tools: core.tools             # packs contribuem no registry desta peça
  runner: y.agent-runner        # usa os serviços Y declarados
  application: opera.execution  # fornece execution.Service para a UI

services declarados
  storage -> opera.store, opera.records
  provider -> y.provider        [requires opera.store, opera.records]
  tools -> y.tool-registry, opera.work [requires opera.records]
  runner -> opera.runner        [requires y.provider, y.tool-registry, opera.store]
  application -> opera.execution [requires opera.runner, opera.store, opera.records]
```

O exemplo descreve direção e ownership, não IDs finais. Validar cada key/serviço
contra o código e SDK fixado antes de tornar o grafo normativa; IDs públicos de
serviço precisam de owner único e versão de contrato explícita.

## Etapas

### 0. Definir o contrato e o resultado ergonômico

- Inventariar os pontos de seleção existentes: providers, search, tool packs,
  modos de runtime, storage, contexto, extensions e inspeção.
- Para cada um, registrar owner, dependências, serviço público, seleção atual,
  lifecycle, nível de confiança e se a substituição é necessária de fato.
- Definir exemplos de composição alvo: padrão, mínima/headless e inspeção, sem
  confundir perfil do agente com perfil de runtime.
- Decidir, com base na API publicada de Y, o que pode ser substituído em startup e
  o que pode ser reconciliado em safe point. Não presumir capacidades não
  publicadas.

**Aceite:** inventário com dependências e trust boundaries; exemplos de perfil
resolvidos sem duplicar seleção entre execução, configuração e descoberta.

### 1. Normalizar a descrição das peças

- Definir um registro/descriptor de composição no core, usando os metadados
  públicos do Y em vez de criar um framework paralelo.
- Cada definição deve declarar ID estável, versão de implementação, serviços
  providos/requeridos, bindings, capabilities e campos de configuração relevantes.
- Resolver dependências e validar IDs duplicados, serviços ausentes, versões
  incompatíveis, capabilities obrigatórias e conflitos antes de iniciar recursos.
- Definir erros que apontem o perfil, a peça e o serviço ou requisito que falhou.
- Manter factories explícitas e instanciadas; sem registro por `init`, globais
  mutáveis ou seleção baseada em ordem de slices.

**Aceite:** o core consegue explicar e validar a composição antes de Start; testes
cobrem grafo inválido, dependência ausente, conflito e implementação substituta.

### 2. Dar uma forma comum à montagem no core

- Trocar a lista montada diretamente em `core.Open` por um compositor que recebe
  um perfil e devolve as `component.Definition` finais.
- Separar descrição/resolução de montagem da inicialização do Harness; manter
  `runtime.Harness` como único dono de Start, safe points, gerações e Close.
- Converter primeiro as peças que já têm fronteira de substituição concreta:
  provider, persistência, catálogo de ferramentas/tool packs e capabilities de
  contexto/search quando o contrato permitir.
- Manter módulos de regras como packages coesos. Só transformá-los em definições
  Y quando realmente fornecem serviço consumido por outras peças.
- Remover caminhos de seleção duplicados para que catálogo, configuração,
  descoberta e execução consultem a mesma composição.

**Aceite:** a composição padrão mantém comportamento e contratos; uma peça pode
ser substituída por uma implementação de teste sem alterar CLI, TUI ou módulo
consumidor.

### 3. Introduzir perfis de composição pequenos e explícitos

- Definir perfis de runtime com composição base e overrides declarativos por ID.
- Começar apenas pelos casos com diferença operacional comprovada, como perfil
  padrão, inspeção e execução sem interface. Não criar uma matriz de combinações
  especulativa.
- Resolver os perfis para um grafo final, validar tudo antes de ativar e imprimir
  uma representação legível para diagnóstico.
- Permitir substituir uma linha inteira por ID, com precedência documentada; não
  adicionar uma linguagem geral de patches nem sobreposição implícita.
- Manter `AgentProfile` distinto de `RuntimeMode` e composição de capabilities.

**Aceite:** `doctor`/inspeção consegue mostrar as peças selecionadas e suas
dependências; os perfis fornecidos são reprodutíveis e não alteram seleção em
tempo de execução por switches dispersos.

### 4. Unificar configuração e catálogo das capabilities

- Mover seleção específica para o descriptor/factory que possui a capability;
  configuração expõe IDs e opções genéricas, sem importar provider concreto.
- Consolidar provider, autenticação e search no mesmo catálogo de provider, e
  integrar tool packs/contexto no compositor somente onde forem serviços
  intercambiáveis.
- Definir defaults explicitamente e rejeitar IDs desconhecidos. Sem fallback
  silencioso ou varredura ambiental que contorne a configuração resolvida.
- Gerar documentação ou saída de diagnóstico do catálogo a partir das mesmas
  definições usadas na composição.

**Aceite:** descoberta, configuração e execução exibem os mesmos IDs e metadados;
um novo provider/tool pack não requer novo switch de provider na UI.

### 5. Fechar lifecycle, reconfiguração e falhas parciais

- Para cada componente, registrar ownership das aquisições em `Scope.Track` ou
  API pública equivalente e associar goroutines a um owner cancelável.
- Em falha de Start/Reconcile, desfazer apenas as novas aquisições e preservar a
  geração ativa; no fechamento, interromper ações, cancelar, drenar e fechar em
  ordem inversa com tentativas para todos os closers.
- Reconfigurações dinâmicas usam exclusivamente safe points/views do Y. Uma
  operação continua presa à mesma geração de ferramentas, handlers, hooks e
  contexto.
- Preservar identidade, aprovação, digest de consentimento e comportamento de
  retomada ao alterar a geração.
- Não prometer hot reload para componentes estáticos. Caso extensão dinâmica
  precise ser reconciliada, documentar e testar apenas o caminho suportado pelo Y.

**Aceite:** testes de falha parcial, rollback, cancelamento, close idempotente,
ordem inversa, drain da geração anterior e rejeição de resposta obsoleta.

### 6. Melhorar o caminho de quem cria uma peça

- Criar um exemplo mínimo de componente compilado com descriptor, serviços,
  configuração, lifecycle, erro contextual e teste com double.
- Documentar “como adicionar uma capability” com os passos exatos: owner/package,
  serviço, definição, inclusão no perfil, configuração, validação e critérios
  para usar componente compilado versus extensão isolada.
- Adicionar checks arquiteturais para impedir imports invertidos, providers
  concretos fora de adapters/composição e definições sem metadados requeridos.
- Expor uma inspeção de grafo útil tanto no desenvolvimento quanto em `doctor`,
  sem imprimir segredos ou valores sensíveis de configuração.

**Aceite:** uma capability de exemplo pode ser adicionada em um único fluxo
documentado sem edição de switches em vários pacotes; os checks falham com
mensagens de correção acionáveis.

### 7. Avaliar distribuição externa como projeto separado

- Após estabilizar IDs, contratos, configuração e lifecycle, avaliar instalação e
  atualização de bundles externos, dependências, assinatura/digest, trust e
  compatibilidade.
- Usar WASM/processo isolado para código não confiável com host APIs mediadas;
  não carregar plugins Go arbitrários no processo como atalho de ergonomia.
- Definir instalação, remoção, rollback e persistência de consentimentos antes de
  permitir ativação automática de módulos externos.

**Aceite/gate:** ADR próprio e revisão de segurança antes de implementar qualquer
gerenciador de plugins externos. Esta etapa não bloqueia a ergonomia para módulos
compilados e composições internas.

## Ordem e estratégia de migração

Executar as etapas 0–6 em mudanças pequenas, mantendo o perfil padrão equivalente
ao comportamento atual. Cada migração deve incluir consumidor, documentação,
testes de contrato e checks de arquitetura na mesma etapa. Evitar uma conversão
em massa de todos os packages: a ordem inicial recomendada é provider/catalog,
tools/tool packs, search/context e persistência, conforme o inventário confirmar
fronteiras reais. Reavaliar `execution` por último; separar loop, sessão,
aprovação ou retomada exige prova de corte acíclico e preservação dos invariantes.

Não modificar schema, IDs duráveis, formatos JSON/JSONL, contratos de CLI ou
HarnessVersion sem necessidade concreta e plano de compatibilidade/recuperação.
Atualizar ADR somente quando a proposta de contrato for aceita; este documento é
um plano, não uma decisão de arquitetura.

## Gates de validação

- `go list ./...` e checks de arquitetura confirmam direção acíclica e owners.
- `make check`, `make lint`, `make matrix`, `make inventory` e `go mod tidy -diff`
  passam sem drift incidental.
- Testes de composição cobrem grafo inválido, capability obrigatória ausente,
  substituição por factory, rollback, cancelamento e fechamento.
- Smokes CLI offline mantêm `--help`, `--version` e `doctor --format jsonl`.
- Testes de recuperação preservam IDs, aprovações, identidade de sessão e
  operações duráveis após mudança de composição.
- Testes live pagos/externos ficam fora sem autorização e credenciais disponíveis.

## Não objetivos

- Fazer Y substituível por outro runtime.
- Converter cada package Go em plugin ou dividir o `go.mod`.
- Reimplementar o loop do agente ou o gerenciador de gerações no Opera.
- Permitir plugins nativos de terceiros no processo ou conceder acesso implícito a
  filesystem, rede, processos ou segredos.
- Adicionar hot reload universal, perfis especulativos ou uma linguagem genérica de
  configuração/patch.
