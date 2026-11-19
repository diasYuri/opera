# Reverter e ramificar histórico

**Tipo:** recuperação e exploração de sessões

**Prioridade potencial:** média para corrigir contextos ruins sem perder o histórico original.

## O que o Codex oferece

O store local de threads mantém histórico de rollout e implementa operações de revert/fork, além de busca e paginação. Uma thread pode ser retomada ou ramificada a partir de um ponto do histórico, preservando a origem. O código separa o append canônico de histórico da escrita de metadados consultáveis.

## Evidência no Codex

- [Thread store](</Users/yuri/git/other/codex/codex-rs/thread-store/README.md>)
- [Implementação de revert](</Users/yuri/git/other/codex/codex-rs/thread-store/src/local/revert_thread.rs>)
- [Fork paginado](</Users/yuri/git/other/codex/codex-rs/thread-store/src/local/paginated_fork.rs>)
- [Histórico e busca](</Users/yuri/git/other/codex/codex-rs/thread-store/src/local/thread_history/>)

## Estado e oportunidade no Opera

O Opera lista/inspeciona/exporta execuções, ramifica checkpoints concluídos e compacta transcrições com arquivamento. Isso não é a mesma UX que selecionar um ponto arbitrário de conversa para continuar uma linha alternativa. Ver [contrato da CLI](../../../docs/cli-contract.md) e [operações](../../../docs/operations.md).

Revert/fork granular poderia ajudar quando uma sessão tomou um rumo ruim. Deve preservar a trilha de origem e não reexecutar chamadas externas ao reconstruir contexto.

## Direção de desenho

- Diferenciar truncar/ocultar contexto, criar branch e desfazer efeitos; só o primeiro é reversível no log.
- Identificar cursor/evento e fronteira de turno exatos, validando tool calls/results pareados.
- Guardar origem e cursor herdado; criar IDs novos para operações/approvals e não herdar autorizações.
- Exigir checkpoint seguro ou completar/reconciliar operações incertas antes de ramificar.
- Arquivar o original antes de qualquer reescrita física; preferir nova geração/branch imutável quando possível.

## Critério de sucesso

Uma branch pode continuar de um ponto selecionado com histórico consistente, sem reproduzir efeitos, reaproveitar approvals ou tornar ambígua a origem dos eventos herdados.
