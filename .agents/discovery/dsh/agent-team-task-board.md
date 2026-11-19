# Quadro de tarefas para equipes de agentes

**Tipo:** recurso de coordenação multiagente

**Prioridade potencial:** alta se o uso de workers passar a envolver divisão de trabalho entre agentes.

## O que o DSH oferece

O DSH permite que um grupo de subagentes compartilhe um quadro durável com tarefas, dependências, responsável, estado e revisões. Agentes reivindicam tarefas com controle de concorrência; o líder pode reatribuir trabalho. O board complementa a árvore de subagentes, em vez de criar uma segunda identidade de membros.

## Evidência no DSH

- [Serviço Agent Team](</Users/yuri/git/other/deepseek-harness/packages/team/agent-team/README.md>)
- [Ferramentas do Agent Team](</Users/yuri/git/other/deepseek-harness/packages/team/tool-agent-team/README.md>)
- [Arquitetura de subagentes e capability seams](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)

## Estado e oportunidade no Opera

O Opera já tem criação, execução, consulta, espera, cancelamento e retomada de filhos, limites compartilhados e comandos `/tasks` e `/agents`. Isso resolve o lifecycle dos workers, mas não equivale a um quadro de trabalho compartilhado com dependências, reivindicação concorrente e responsável. Ver [contrato da CLI](../../../docs/cli-contract.md) e [configuração de limites](../../../docs/configuration.md).

Um board daria visibilidade sobre quem está trabalhando em quê e permitiria dividir trabalhos maiores sem depender apenas de mensagens entre agentes.

## Direção de desenho

- Usar a identidade e a descoberta de filhos já fornecidas pelo runtime Y; não manter um roster paralelo.
- Persistir fatos do board e derivar a view, com revisão esperada/CAS para rejeitar atualizações obsoletas.
- Manter atribuição como coordenação, nunca como autorização para editar arquivos ou executar ferramentas.
- Começar com hints de arquivos e avisos de sobreposição. Bloqueio real exige isolamento/worktrees e tratamento de comandos que escrevem fora das ferramentas instrumentadas.
- Definir o comportamento em falha de persistência: uma mutação visível sem durabilidade confirmada não pode parecer concluída.

## Critério de sucesso

Uma tarefa só pode ser reivindicada quando dependências estão concluídas; updates concorrentes obsoletos falham com estado atual; recuperação reconstitui o mesmo board sem alterar a identidade dos filhos nem autorizar efeitos.
