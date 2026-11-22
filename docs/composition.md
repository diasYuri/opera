# Composição do Opera

O Opera usa o SDK Y v1.4.0. Em `internal/core`, um perfil escolhe registros do
catálogo compilado. Cada registro produz uma `component.Definition`. O
`runtime.NewHarness` valida bindings, versões de serviço, conflitos e ciclos e
devolve os descriptors na ordem de inicialização. `Harness.Start` ativa o grafo.
Use `opera doctor` ou `opera doctor --format jsonl` para inspecionar o mesmo
grafo sem ativá-lo.

## Perfis e precedência

| Perfil | Componentes | Entrada |
| --- | --- | --- |
| `default` | Storage, startup-settings, execution-state, provider, tools, runner e application | Chat interativo |
| `headless` | Mesmo grafo de execução de `default` | Run, worker, resume e ações de CLI |
| `inspection` | Storage, startup-settings e inspection | Sessions, inspeção de uso e checagem persistida de extensões |

`headless` seleciona a mesma execução durável por outra entrada. A UI pertence
à CLI e fica fora do grafo. `inspection` evita a abertura de provider,
registry de ferramentas, skills e extensões. O modo de execução escolhido por
`RuntimeMode` e a especialização de prompt `AgentProfile` continuam sendo
dimensões do run, conforme o [ADR 010](adr/010-agent-profiles-and-runtime-modes.md).

O perfil fornece os IDs padrão. Uma seleção explícita em
`Options.Selections[StorageSlot]` substitui toda a implementação de storage.
Use `DefaultComponentCatalog` com o registro adicional para estender o catálogo
da composição. O slot tem uma seleção efetiva. IDs ausentes ou fora do slot
falham na resolução. Os overrides de composição são opções Go do core.

Provider de modelo e suas credenciais continuam seguindo a configuração
resolvida descrita em [Configuração](configuration.md). A instância do
catálogo de providers é usada por login, descoberta, execução e busca nativa.
A busca usa o adapter nativo do provider selecionado quando disponível, salvo
a restrição de ativação por configuração do projeto. A opção explícita
`native` exige disponibilidade. `brave` usa seu próprio resolver de credencial
e `disabled` desativa a contribuição de busca. Os valores e chaves de
`search.options.*` permanecem compatíveis.

Na retomada, `startup-settings` lê a metadata uma vez para montar a
composição, verifica o workspace e preenche provider, modelo, response format e
anexos quando suas opções estão ausentes. O caminho normal de execução
verifica também a identidade, o snapshot, a geração de ferramentas e a
aprovação antes de continuar. Defaults provenientes do checkpoint ficam no
serviço de startup daquela composição. Um comando administrativo recuperado
também restaura as opções administrativas a partir da metadata.

O diagnóstico apresenta os perfis, IDs, versões, serviços requeridos e
fornecidos, bindings explícitos, capabilities, escolhas de provider e tool
packs. A proveniência mostra a chave e a camada da configuração. Valores
como modelo, endpoint, caminho e referência de credencial ficam de fora.
Falhas de seleção de provider ou busca aparecem como erro de composição
offline. Antes do onboarding, o grafo pode ser inspecionado com o provider
marcado como não configurado. O diagnóstico pode ler os arquivos de
configuração e verificar o PATH. Ele não abre o banco, resolve credenciais,
inicia ferramentas ou consulta endpoints.

## Inventário e ownership

| Capacidade | Owner e dependências | Seleção e lifecycle | Confiança |
| --- | --- | --- | --- |
| Persistência | Core abre Y SQLite para `opera.store@1.0.0` e SQLite Opera para `opera.records@1.0.0` | Slot de startup; interfaces de consumo em `execution`; fecha após os consumidores | Adapters compilados com acesso ao diretório de dados; cleanup preserva dados |
| Settings efetivos | Core fornece `opera.startup-settings@1.0.0`, requer storage na retomada | Perfil e configuração de entrada; publica dados efetivos depois da validação do checkpoint | Informações privadas de composição, sem projeção de valores no diagnóstico |
| Provider, login e modelos | Adapters de provider e `modules/providers/catalog`; componente fornece a key pública de provider do Y e usa stores para accounting | Seleção pelo catálogo; factory adquire cliente no Start e scope fecha | Processamento remoto pela configuração e credencial resolvidas |
| Busca | Busca nativa no catálogo de provider ou adapter Brave; tools consome o contrato Y `search.Provider` | Capacidade do pack `web-search`, factory selecionada no startup; credencial lazy na chamada | Rede externa autorizada por configuração do usuário, política e aprovação da ferramenta |
| Estado de execução e contexto | `execution-state` monta `execution.Service`, `Work`, política, broker, skills e fonte de anexos sobre settings e stores | A aplicação completa a montagem antes de publicar o serviço. O estado de anexos tem sincronização própria. Contexto de arquivo e skills usam seus adapters | Workspace delimitado; dados externos passam pelas fronteiras de contexto e efeito |
| Ferramentas e tool packs | `tools` fornece registry público do Y e `opera.extensions@1.0.0`; depende do estado de execução e settings | Lista de packs compilados, contribuições adicionais e desabilitação por ID; registry possui as inscrições e lifecycles associados | Código compilado confiável; ferramentas continuam sujeitas a política, aprovação e limites |
| Extensões | `modules/extensions` usa o Host e runtime views públicos do Y | MCP, WASM e memória opcionais; configuração privada e consentimento por digest; o scope fecha o Host antes do registry | WASM com APIs mediadas, MCP via transporte explícito. Um subprocesso stdio tem os privilégios do usuário do processo |
| Runner e execução | Runner Y usa provider, registry, estado, extensões e store. `application` publica `opera.execution@1.0.0` | A aplicação cancela e drena runs antes de fechar runner e stores. Agent profiles e runtime modes continuam no módulo de execução | SDK e aplicação compilados, com políticas e identidades duráveis |
| Inspeção de registros | `inspection` implementa a mesma publicação de serviço sobre storage e settings | Perfil separado, sem inicialização de execução ou extensões | Leitura de dados locais pelo workspace existente |

## Adicionar uma capacidade compilada

1. Defina o owner. Regras de produto ficam em `internal/modules` e efeitos
   externos em `internal/adapters`. Use componente para serviço próprio,
   aquisição/teardown ou substituição real. Para acrescentar ferramentas ao
   mesmo registry, adicione um `ToolPack` com ID e registro explícitos. Para
   um provider, acrescente a `Entry` do adapter ao catálogo de providers,
   inclusive o login e a busca nativa quando forem oferecidos.
2. Declare o contrato no core com a key pública existente do Y ou uma
   `component.Key[T]` de owner único. Dê à implementação um ID e versão e
   mantenha versões de serviço independentes. Use `Provides` e `Requires`
   para serviços com um fornecedor no perfil. Use `Requirements` do Y quando
   precisar selecionar explicitamente um fornecedor e sua faixa de contrato.
3. Registre o builder em `DefaultComponentCatalog` e inclua a peça em
   `selectRuntimeProfile`, ou passe o registro adicional para ocupar um slot
   existente. Declare as chaves relevantes para proveniência segura. Resolva
   IDs e defaults na composição. Receba só os dados necessários da config.
4. Construa a definição sem abrir arquivos, conexões ou workers. No `Start`,
   resolva cada serviço com `component.Require`, adquira seus recursos,
   registre imediatamente fechamento com `Scope.Track` ou workers com
   `Scope.Go` e publique com `component.Provide`. Contextualize falhas e
   preserve seus erros. O SDK valida o grafo e faz o rollback das aquisições.
5. Resolva o perfil em teste antes de ativá-lo. Verifique um fluxo real com
   double, falha parcial, cancelamento e fechamento. Rode `make check` para
   validar a direção de imports e testar os contratos dos perfis. Confira o
   resultado offline com `opera doctor --format jsonl`.

O [exemplo de storage](../internal/core/composition_storage_example_test.go)
registra `storage.fixture`, abre checkpoints e records em um banco alternativo
no Start, retorna erros de aquisição contextualizados e fecha as conexões em
ordem inversa. O teste seleciona o slot, inicia um provider de script, chega a
uma aprovação, fecha o runtime e retoma com a mesma sessão e aprovação. Também
exercita grafos inválidos antes da invocação da factory.

Use os contratos de [extensões](extensions.md) para integração isolada, com
grants e consentimento explícitos. Um componente compilado roda no processo e
precisa do mesmo nível de confiança que o próprio Opera.

## Mudanças de geração e fechamento

O `runtime.Harness` publicado é estático. Uma troca interativa de modelo ou
thinking prepara outro Harness no ponto ocioso e mantém o anterior se a
ativação do candidato falhar. O novo Harness reabre os recursos de startup.
Um adapter que não suporte aquisições simultâneas pode exigir fechar a sessão
e iniciar outra. Quando a troca já foi aplicada, um erro de fechamento do
anterior é reportado como erro de limpeza pós-troca.

O reload dinâmico existente passa exclusivamente pelo Host do Y. A edição
compatível é um toggle de uma ativação WASM já descoberta. A checagem de
idle e de aprovações duráveis ocorre antes do reload. Views adquiridas retêm
tools, handlers, hooks e contexto daquela geração até a liberação. Uma falha
pré-commit conserva a geração anterior. Um erro de drenagem pós-commit conserva
a nova geração e o Y continua a limpeza em background. O fingerprint
persistido continua derivado dos descriptors e do digest de configuração, sem
alterar schema, JSONL de execução, IDs duráveis ou versão do Harness.

MCP/memória, mudanças de diretórios ou de bytes/grants de WASM aquecido exigem
um novo escopo CLI. A [avaliação de distribuição externa](adr/011-external-bundle-distribution.md)
registra o que precisa ser definido e revisto antes de construir um instalador.
