# Store canônico separado de metadados consultáveis

**Tipo:** padrão de persistência e índice
**Aplicabilidade:** média-alta para evolução de catálogo, busca e recuperação de sessões.

## Padrão no Codex

O `ThreadStore` separa append de histórico canônico de atualização explícita de metadados. O writer vivo (`LiveThread`) coordena append e patches; arquivos JSONL guardam o rollout, enquanto SQLite mantém metadados consultáveis quando disponível. A camada de core usa handles e não precisa saber qual backend persiste os dados.

## Evidência no Codex

- [Thread Store: responsabilidades e direção](</Users/yuri/git/other/codex/codex-rs/thread-store/README.md>)
- [Trait do store](</Users/yuri/git/other/codex/codex-rs/thread-store/src/store.rs>)
- [LiveThread writer](</Users/yuri/git/other/codex/codex-rs/thread-store/src/live_thread.rs>)
- [Persistência local e busca](</Users/yuri/git/other/codex/codex-rs/thread-store/src/local/>)

## Aplicação no Opera

O Opera já separa eventos/checkpoints de projeções e armazenamento SQLite; suas notas de persistência preservam formatos e recovery. Esse modelo é útil para catálogos grandes ou buscas sem carregar transcript completo. Ver [operações e recuperação](../../../docs/operations.md) e [plano arquitetural](../../../docs/refactoring-plan.md).

## Regras úteis

- A append API de eventos não deve inferir ou mutar metadados semânticos escondidos.
- Escrever histórico e metadados deve ter ordenação/atomicidade documentada, com tratamento de falhas parciais.
- Índices são reconstruíveis ou compatíveis com um fallback explícito; falha de índice não deve ser confundida com lista vazia.
- Núcleo de execução depende de contrato de store, não de JSONL/SQLite concretos.
- Migrações não devem reescrever gerações duráveis sem necessidade e política de recuperação.

## Limite

Não duplicar stores para separar campos que compartilham lifecycle e consistência. O corte só compensa quando o dado consultável e o histórico têm semânticas/consumidores distintos.
