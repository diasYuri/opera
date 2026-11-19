# App Server como API de controle

**Tipo:** recurso de integração e automação

**Prioridade potencial:** alta quando houver demanda de editores, clientes externos ou automação de múltiplas sessões.

## O que o Codex oferece

O Codex separa o core de execução de clientes como TUI e editor por um App Server JSON-RPC. O protocolo tem operações e notificações tipadas para threads, turns, itens, approvals, configuração, modelos, ferramentas e eventos de lifecycle. O cliente pode iniciar/retomar threads, enviar input, acompanhar streaming e responder a decisões sem interpretar o output textual do CLI.

## Evidência no Codex

- [App Server](</Users/yuri/git/other/codex/codex-rs/app-server/README.md>)
- [Protocolo App Server](</Users/yuri/git/other/codex/codex-rs/app-server-protocol/src/lib.rs>)
- [Cliente App Server](</Users/yuri/git/other/codex/codex-rs/app-server-client/README.md>)
- [Integração do core com UIs Rust](</Users/yuri/git/other/codex/codex-rs/core/README.md>)

## Estado e oportunidade no Opera

O Opera oferece CLI/TUI e saída JSONL de execução, mas não um servidor de controle persistente com protocolo tipado para sessões. O runtime Y já é a base de execução; uma API de aplicação poderia abrir essa capacidade a editores e outras UIs sem criar outro loop. Ver [contrato da CLI](../../../docs/cli-contract.md) e [plano arquitetural](../../../docs/refactoring-plan.md).

## Direção de desenho

- Definir métodos, eventos, IDs, negociação de capabilities, erros e versionamento em um protocolo separado da TUI.
- Reusar `execution.Service`, stores, approvals, checkpoints e política do Opera.
- Tornar explícitas as semânticas de reconnect, replay, cursores, cancelamento e respostas obsoletas.
- Isolar transporte/autenticação em adapter; clients não devem ter acesso às implementações de provider/persistence.
- Manter servidor e TUI como superfícies diferentes sobre os mesmos casos de uso.

## Critério de sucesso

Um cliente inicia, retoma e controla execução, recebe eventos identificáveis e resolve approvals usando o mesmo comportamento durável da CLI, inclusive após desconexão e reconexão.
