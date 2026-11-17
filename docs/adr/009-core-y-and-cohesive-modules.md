# ADR 009: Core baseado em Y, módulos coesos e adapters externos

- Status: aceito
- Data: 2026-11-17
- Substitui parcialmente: ADR 007 para a organização de módulos internos

## Contexto

A arquitetura anterior descreveu `app`, `harness` e portas/adapters com limites
genéricos. Na prática, o SDK Y é a fundação escolhida do Opera e várias capacidades
internas colaboram naturalmente. Abstrair Y como provider ou exigir uma interface
entre cada módulo esconderia dependências reais e multiplicaria wrappers. Ao mesmo
tempo, providers externos e efeitos concretos precisam ficar isolados e
substituíveis.

## Decisão

1. O runtime público Y é dependência de primeira classe do `internal/core`. A
   composição, o lifecycle de `runtime.Harness`, componentes, bindings e resolução
   usam diretamente suas APIs públicas. Não há `YAdapter` nem runtime paralelo.
2. Código de produto é organizado em `internal/modules/<capacidade>` por ownership
   e coesão. Módulos colaboram diretamente quando a relação é acíclica. O primeiro
   grupo `execution` inclui `Service` e `Work` enquanto ambos mantiverem chamadas e
   estado mutuamente ligados.
3. Integrações externas e efeitos substituíveis ficam em
   `internal/adapters/<tipo>/<submódulo>`. Cada provider possui um package próprio
   em `internal/adapters/providers/<provider>` que controla seu cliente,
   autenticação, metadados e factory.
4. `internal/ui/cli` adapta a interface de processo e chama a composição; `internal/ui/tui`
   apresenta estado e emite intenções. A UI não importa implementação de provider,
   persistência ou runtime.
5. O core é o composition root e pode depender de módulos/adapters concretos. A
   direção inversa não é permitida. Portas são usadas nas fronteiras de efeitos ou
   substituição reais, não entre todo par de módulos.
6. Mantemos um `go.mod`; não criamos submódulos Go separados por capability.

## Consequências

- As antigas packages `internal/app` e `internal/harness` são migradas por etapas
  para `internal/modules` e `internal/core`, atualizando consumidores no mesmo
  corte. Facades temporárias só podem existir durante uma etapa validada e devem
  ser removidas ao final.
- Módulos não dependem do core nem do composition root. Uma relação recíproca é
  sinal para manter o código no mesmo módulo, não para adicionar interfaces
  artificiais.
- A composição mantém factories explícitas e usa o catálogo padrão configurado;
  nenhuma lista de providers se espalha em CLI, UI ou módulos.
- Compatibilidade durável, lifecycle do Y, autorização e protocolos permanecem
  invariantes durante a migração.

## Alternativas recusadas

- Tratar o runtime Y como adapter: isso esconderia a dependência fundacional do
  produto e sugeriria substituir uma escolha arquitetural que é fixa.
- Forçar ports and adapters entre todas as capabilities: adiciona indirection sem
  efeito substituível e torna a navegação do código mais difícil.
- Deixar autenticação e protocolos de provider em CLI/UI: mistura apresentação com
  detalhes externos e espalha decisões de provider.
- Criar um módulo Go independente por provider ou por módulo de produto: não há
  necessidade atual de versão/build separado.

## Verificação

`docs/refactoring-plan.md` define as etapas. O check de arquitetura deve comprovar
que providers concretos só são importados por adapters/composição, módulos não
importam core/UI, e UI não importa adapters. A validação completa deve registrar
`go list ./...`, `make check`, lint, matrix, race e smokes offline.
