# Refactoring validation

Data: 2026-11-16. Este registro separa o que foi implementado do que continua
pendente; o plano de refatoração não está concluído.

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

## Trabalho pendente

- Etapa 3 está parcial: Work e Extensions agora são serviços declarados e tool
  packs são substituíveis, mas `harness.Open` ainda contém a composição principal
  em uma função extensa. Storage, provider, tools, runner, application e inspection
  ainda precisam ser extraídos em builders/componentes coesos. Também falta testar
  rollback/cleanup de falha parcial com doubles focados nessa composição.
- Etapa 4 está parcial: atividade tem submodelo próprio; composer, seleção/painel
  e conversação continuam coordenados pelo model shell de `internal/ui/chat.go`.
  A extração de ownership desses fluxos e os testes completos de foco, cancelamento,
  geração obsoleta, resize e modos acessíveis continuam pendentes.
- A etapa 5 adicionou regras e testes do verificador, documentação de tool packs e
  exemplo executável. Ainda não há exemplo de substituição de provider com uma
  implementação funcional própria; o exemplo atual demonstra substituição de
  tool pack.

Essas lacunas são concretas; portanto, este relatório não declara o plano completo.

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
O trabalho restante descrito acima continua necessário.
