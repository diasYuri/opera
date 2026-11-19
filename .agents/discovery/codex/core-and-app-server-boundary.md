# Core reutilizável e protocolo de aplicação

**Tipo:** padrão de arquitetura de produto
**Aplicabilidade:** alta se Opera ganhar novos clientes além de CLI/TUI.

## Padrão no Codex

O `codex-core` concentra lógica de negócio para UIs Rust, enquanto o App Server e seu protocolo permitem que clientes controlem threads e recebam atualizações. O TUI e o App Server compartilham o core; transporte e interface não definem um segundo agente.

## Evidência no Codex

- [Responsabilidade do core](</Users/yuri/git/other/codex/codex-rs/core/README.md>)
- [App Server](</Users/yuri/git/other/codex/codex-rs/app-server/README.md>)
- [Protocolo](</Users/yuri/git/other/codex/codex-rs/app-server-protocol/src/lib.rs>)
- [Cliente de protocolo](</Users/yuri/git/other/codex/codex-rs/app-server-client/README.md>)

## Aplicação no Opera

O Opera já separa `internal/modules/execution` e `internal/ui/{cli,tui}`, e monta o runtime Y em `internal/core`. Esse padrão reforça a possibilidade de adicionar outro frontend/transport sem mover regras para UI ou duplicar o runner. Ver [plano de refatoração](../../../docs/refactoring-plan.md).

## Regras úteis

- Um core de casos de uso serve múltiplos clientes; cliente traduz interação e apresentação.
- API pública tipada deve representar lifecycle, errors e streaming, não espelhar structs internas.
- Protocolo e transporte versionam independentemente da implementação de UI.
- Capability de host é exposta seletivamente; conexão não concede autoridade por si só.

## Limite

O Opera já possui essa direção arquitetural. A descoberta serve como validação e referência para uma futura API, não como proposta de criar um novo pacote `core` ou trocar o runtime Y.
