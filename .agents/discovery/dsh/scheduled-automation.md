# Agendamento de automações

**Tipo:** execução recorrente e integração operacional

**Prioridade potencial:** média; requer decisão explícita sobre execução fora da CLI foreground.

## O que o DSH oferece

O DSH permite criar, editar, pausar, retomar e executar lembretes agendados, com regras recorrentes, timezone, destino de Session e histórico de entregas. O scheduler pertence ao Host em execução; a persistência da tarefa não significa execução enquanto o Host está parado. A entrega confirma enfileiramento da mensagem, não conclusão do trabalho pelo agente.

## Evidência no DSH

- [Guia de Schedule](</Users/yuri/git/other/deepseek-harness/docs/user/guide/schedule.md>)
- [Pacote Schedule](</Users/yuri/git/other/deepseek-harness/packages/schedule/schedule/README.md>)
- [Arquitetura Host/Client](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)

## Estado e oportunidade no Opera

O contrato do Opera mantém execução em foreground e não destaca jobs ao encerrar a CLI. Não há scheduler persistente documentado. Ver [contrato da CLI](../../../docs/cli-contract.md). Lembretes recorrentes podem ser úteis para verificações de CI, relatórios e tarefas locais, mas introduzem um processo que precisa permanecer ativo.

## Direção de desenho

- Só avançar junto com uma decisão de produto sobre serviço local/daemon ou integração externa que acione invocações foreground.
- Persistir regra, versão, timezone, próxima ocorrência, destino e recibos; separar “entregue ao inbox” de “execução concluída”.
- Definir semântica de restart, ocorrências perdidas, duplicação, concorrência e cancelamento antes de prometer exatamente uma vez.
- Reusar `execution.Service`, policy e limites de workspace; não contornar aprovação por ser tarefa agendada.
- Fornecer comandos para listar, pausar, executar agora, cancelar e inspecionar histórico sem iniciar provider.

## Critério de sucesso

Após restart, o scheduler retoma com política de catch-up definida, não duplica silenciosamente efeitos e mostra claramente se cada ocorrência foi agendada, entregue, executada ou falhou.
