# Hooks de lifecycle

**Tipo:** extensibilidade e automação local

**Prioridade potencial:** média se integrações precisarem reagir a fases do run sem alterar o core.

## O que o Codex oferece

O Codex tem eventos de hook em fases como início/fim de sessão, envio de prompt, requisição ao modelo, execução de ferramenta, aprovação e compactação. O engine descobre configurações, valida schemas de input/output, aplica confiança e executa comandos/MCP conforme o tipo de hook. Existem regras para ignorar hooks não gerenciados em modo administrado.

## Evidência no Codex

- [Engine e dispatcher de hooks](</Users/yuri/git/other/codex/codex-rs/hooks/src/engine/dispatcher.rs>)
- [Tipos e schemas](</Users/yuri/git/other/codex/codex-rs/hooks/src/types.rs>)
- [Eventos de execução](</Users/yuri/git/other/codex/codex-rs/hooks/src/events/>)
- [Hooks empacotados](</Users/yuri/git/other/codex/codex-rs/plugin/src/bundled_hooks.rs>)

## Estado e oportunidade no Opera

O Opera já usa os protocolos/extensões de Y e permite algumas contribuições limitadas por WASM, incluindo hooks previstos pelo runtime. Não possui necessariamente o catálogo de lifecycle configurável por comandos externos do Codex. Ver [extensões Opera](../../../docs/extensions.md) e [UI extensions](../../../docs/ui-extensions.md).

Hooks podem integrar lint, auditoria, política corporativa ou notificações. Seu uso cruza fronteiras de execução e pode virar uma via de bypass se o hook puder alterar decisões sem autorização.

## Direção de desenho

- Preferir hooks públicos do Y quando cobrem o caso; evitar um dispatcher paralelo no Opera.
- Tipar evento, payload, timeout, ordem e capacidade de bloquear/transformar o fluxo.
- Separar hook observacional de hook decisório; exigir consentimento e política para executar processos externos.
- Validar input/output na fronteira de processo e preservar erro, cancelamento e identidade do run.
- Definir policy administrada que possa restringir hooks de usuário/projeto sem ignorar silenciosamente a configuração.

## Critério de sucesso

Um hook autorizado recebe contexto mínimo e correlacionado, termina dentro de limites e não consegue conceder permissões ou modificar resultados duráveis sem passar pelos checks do executor.
