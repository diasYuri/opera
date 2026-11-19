# Worktrees isolados para agentes

**Tipo:** isolamento de trabalho concorrente

**Prioridade potencial:** alta se o uso de workers paralelos provocar conflitos no workspace compartilhado.

## O que o Codex oferece

O Codex tem um crate de worktree com operações Git, metadados e settings, integrado a ferramentas de execução e navegação na TUI. Worktrees permitem que sessões ou agentes trabalhem em diretórios Git separados e que a pessoa inspecione/integre alterações depois. A implementação trabalha sobre os limites do Git e não implica que todos os efeitos externos do agente sejam isolados.

## Evidência no Codex

- [Worktree Git](</Users/yuri/git/other/codex/codex-rs/worktree/src/git.rs>)
- [Paths e metadados de worktree](</Users/yuri/git/other/codex/codex-rs/worktree/src/paths.rs>)
- [Integração com execução](</Users/yuri/git/other/codex/codex-rs/exec/src/worktree.rs>)
- [Browser de worktrees na TUI](</Users/yuri/git/other/codex/codex-rs/tui/src/worktree_browser.rs>)

## Estado e oportunidade no Opera

Os subagentes/workers do Opera compartilham o workspace e usam uma fila comum para serializar escritas instrumentadas; a documentação ressalta que essa fila não oferece isolamento lógico nem controla escritores externos. Ver [políticas e recuperação](../../../docs/operations.md) e [limites de configuração](../../../docs/configuration.md).

Uma opção de worktree por child poderia reduzir colisões de arquivos durante execução paralela e facilitar revisão antes de integrar alterações.

## Direção de desenho

- Definir se o isolamento é por worker, task ou run e como um filho recebe commits/estado inicial do pai.
- Mostrar diff, branch e destino de integração; merge/rebase deve ser uma operação explícita revisável.
- Controlar criação, cleanup e retomada sem apagar trabalho não integrado do usuário.
- Não apresentar worktree como sandbox: rede, credenciais, processos, diretórios externos e MCP continuam precisando de policy.
- Tratar repositórios sem Git, worktrees preexistentes e conflitos de merge como caminhos suportados ou erros explícitos.

## Critério de sucesso

Dois workers podem alterar arquivos em paralelo sem sobrescrever o workspace do outro; o usuário inspeciona cada diff e decide como integrar, e cleanup preserva qualquer alteração ainda não integrada.
