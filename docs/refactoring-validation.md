# Refactoring validation

Atualizado em 2026-11-17. Este registro separa evidências executadas e limites; as etapas 3–5 foram concluídas nos commits desta atualização.

## Alterações e commits

- `d3c2fec refactor: add provider and application capability ports`: catálogo
  imutável por instância, factories nativas fora do harness genérico, IDs
  customizados na configuração, catálogo compartilhado por execução e seletores,
  portas de execução/registros/skills/contexto e clipboard implementado no adapter
  CLI. O caminho de inspeção continua offline sem criar provider.
- `89125e4 refactor: compose tool packs through declared services`: seleção e
  contribuição de tool packs, Work e Extensions publicados como serviços Y com
  dependências declaradas, DTOs de registros sem I/O e regra arquitetural testada.
  `Example_customToolPack` é executável por `go test`.
- `75ce931 refactor(ui): isolate activity animation state`: submodelo possui a
  animação de atividade e invalida frames antigos.
- `fb251f0 test: check tool pack cleanup errors`: fecha os doubles/exemplos de
  teste verificando erros.
- `3963e46 refactor(app): keep skill catalog contracts I/O free`: move DTOs e
  busca de skills para `internal/skillcatalog`, sem importar o pacote de I/O em
  `app`; valida erro quando uma factory retorna provider com ID divergente.

Os contratos e instruções da etapa 0 já estavam no commit `27bf833` antes desta
implementação.

## Evidência executada

- `make check`: passou após a migração. Inclui formato, fronteiras de arquitetura,
  verificação de módulos, vet, testes, consumidor SDK e build CGO-free. O primeiro
  baseline falhava pela importação de `os/exec` na UI; a leitura do clipboard agora
  usa `ui.ClipboardImageProvider`, implementado pelo adapter CLI.
- `make lint`: passou com 0 issues. Um lint no teste TUI previamente alterado pelo
  usuário exigiu simplificar uma condição; essa edição ficou sem commit porque o
  arquivo também contém o teste local preexistente.
- `make matrix`: passou para build e compilação de testes de Linux, macOS e Windows
  em amd64 e arm64. Isso é cross-compilation, não execução nativa nesses sistemas.
- `GOTOOLCHAIN=go1.27.4 GOWORK=off CGO_ENABLED=1 GOFLAGS='-tags=feature_lsp,feature_wasm_ext' go test -race ./...`: passou.
- `make inventory`: passou, índice de 1.746 declarações públicas de Y v1.3.4.
- `go mod tidy -diff`: sem diferenças.
- CLI offline: `bin/opera --help`, `--version` e `doctor --format jsonl` passaram;
  doctor não iniciou provider nem leu credenciais.
- Testes live pagos/externos: não executados; exigem credenciais/autorização.

O código usa `github.com/diasYuri/y v1.3.4` fixado em `go.mod`, compila com o
consumidor SDK em `make check` e não usa `replace`, `go.work` ou imports privados.
As referências locais em `../y/docs` foram consultadas como direção arquitetural;
as garantias consumidas são as APIs públicas compiladas dessa versão.

## Etapas 3–5 concluídas nesta atualização

- `082c1b4 refactor(harness): harden startup and provider catalog composition`:
  fonte de caminhos de contexto atualizável e explícita, validação contra factories
  nulas e providers incompatíveis, inspeção sem provider, e catálogo customizável
  também no comando `models`. Testes cobrem rollback de provider e recurso de tool
  pack, ordem inversa e fechamento único, startup cancelado, caminhos atualizados,
  inspeção offline e provider de catálogo próprio executando pelo `app.Service`.
- `3a3b0ed refactor(ui): give composer overlays and conversation state owners` e
  `b9b4458 refactor(ui): move conversation transitions into owners`: shell usa os
  owners de composer/histórico, seleção/painel e conversação. Identidade de entradas,
  índices, reset e commit de checkpoint pertencem à conversação; transições do
  composer e exclusividade seleção/painel pertencem aos respectivos owners. Os testes
  de owner cobrem navegação de histórico, identidade estável, deduplicação de
  checkpoint e troca/dismissal de painel. Regressões existentes cobrem foco após
  Escape, cancelamento, scrollback, resize, NO_COLOR, no-motion e eventos obsoletos.
- `81c7874 refactor(harness): split component builders by responsibility`:
  `Open` foi reduzido a bootstrap curto; storage, provider, tools, runner,
  application e inspection são builders separados, com keys centralizadas em
  `keys.go` e dependências descritas nos descriptors Y. O request hook não captura
  mais o Service por comunicação lateral. Runtime e lifecycle continuam usando
  `runtime.Harness` público de Y.
- `9c5d78b fix(harness): close providers returned with factory errors`: se uma
  factory retorna provider junto com erro, a composição fecha o recurso parcial; o
  teste comprova fechamento único.
- Um teste de harness demonstra substituição por um provider scriptado registrado em
  `providercatalog.Catalog`, sem rede, credenciais ou alteração do core. A factory
  legada continua aceitando os doubles existentes com ID `openai`.

## Evidência final executada em 2026-11-17

- `rtk proxy make check`: passou após a extração dos builders, incluindo testes,
  verificador de arquitetura, consumidor SDK e build CGO-free.
- `rtk proxy make lint`: passou, 0 issues.
- `rtk proxy make matrix`: passou para Linux/macOS/Windows amd64/arm64, incluindo
  build e compilação dos testes de plataforma. É cross-compilation, não execução
  nativa nesses seis ambientes.
- `rtk proxy make inventory`: passou; 1.746 declarações públicas de Y v1.3.4.
- `rtk proxy go mod tidy -diff`: sem diferenças.
- `rtk proxy env GOTOOLCHAIN=go1.27.4 GOWORK=off CGO_ENABLED=1 GOFLAGS='-tags=feature_lsp,feature_wasm_ext' go test -race ./...`: passou no host macOS arm64.
- Smoke offline passou para `bin/opera --help`, `bin/opera --version` e
  `bin/opera doctor --format jsonl`; doctor não iniciou providers nem leu segredos.
- Nenhum teste live pago/externo foi executado.
- `go test ./internal/ui` passou também para o índice de UI preparado isoladamente,
  usando os arquivos `HEAD` como base para separar alterações prévias. Os gates
  completos foram executados no repositório após a composição final. Mudanças locais
  prévias em `DESIGN.md`, `app/user_question.go`, `chat.go`, `conversation.go`,
  `layout.go`, `layout_test.go`, `theme.go` e `openai_serialization_test.go` ficaram
  fora dos commits desta tarefa.

A implementação usa somente o SDK público Y v1.3.4 fixado em `go.mod`; não adiciona
`replace`, `go.work` nem imports privados. Testes live continuam dependendo de
credenciais e autorização e não fazem parte desta validação.

## Revisão integrada do agente principal

Após a primeira entrega parcial, a revisão encontrou leitura direta do filesystem
na preparação de aprovações e dependência dos casos de uso no pacote de I/O de
contexto. A leitura passou à porta `app.ApprovalFileReader`, implementada em
`internal/approvalfiles` e injetada no Service e Guard pela composição. Dados e
projeções puras de contexto passaram para `internal/contextdata`; o adapter mantém
aliases compatíveis. Regras de arquitetura agora impedem o retorno dessas
dependências concretas ao app.

`make check` passou após a correção, incluindo regressões de fingerprint,
capacidade ausente, cancelamento, confinamento do workspace e tamanho do preview.
As lacunas então identificadas foram concluídas na segunda implementação e na revisão final abaixo.

## Revisão final independente e correções

O agente principal revisou os contratos, builders, propagação do catálogo, owners
TUI e testes de falha, além de executar os gates após a segunda entrega do LUNA.
A revisão corrigiu três falhas adicionais:

- `/context` e a restauração de caminhos no resume agora atualizam a mesma fonte
  consultada pelo request hook. Regressão cobre comando, checkpoint e reabertura.
- O catálogo fecha providers devolvidos com erro e preserva erros de factory e
  cleanup. O catálogo injetado na CLI agora segue também para execução/compactação
  e chat, com teste headless usando provider próprio.
- Descoberta de arquivos/skills do autocomplete saiu da UI para adapter CLI por
  `ComposerCompleter`. `tea.Cmd` executa a descoberta; Update recebe resultados
  identificados por geração e posição/texto, cancela pesquisas substituídas e
  descarta respostas obsoletas. O adapter confina leitura com `os.OpenRoot`.

Os builders foram separados em `component_{storage,provider,tools,runner,
application,inspection}.go`, sem o helper que retornava oito dependências em tupla.
O commit `9e6505e` reúne as correções de catálogo/contexto e a organização dos builders.

A validação do conteúdo preparado para commit em um diretório temporário separado
revelou JSON inválido no schema `user_question` original: os testes do workspace
passavam usando a correção local preexistente do usuário. Foi preparada apenas a
correção sintática mínima (uma chave de fechamento) e um teste de schema válido.
A alteração local mais ampla de compatibilidade strict permanece sem commit.
Após isso, `make check` passou também sobre o snapshot preparado, sem depender de
nenhuma alteração prévia do usuário. Nenhum arquivo do workspace foi substituído
para executar essa verificação.

Gates repetidos pelo agente principal: `make check` no workspace e no snapshot de
commit; `make lint`; `make matrix`; `make inventory` sem drift; `go mod tidy -diff`
sem drift; e `go test -race ./...` com toolchain/tags fixadas e CGO habilitado.
As limitações de cross-compilation e ausência de testes live permanecem as mesmas.
