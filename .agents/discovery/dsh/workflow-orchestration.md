# Workflows de orquestração

**Tipo:** coordenação programática de agentes

**Prioridade potencial:** média, sujeita a exemplos reais de fan-out/merge.

## O que o DSH oferece

O DSH fornece um workflow engine que executa scripts de orquestração e delega para subagentes. O tool `workflow` permite fan-out; o runtime PTC executa scripts em processo Node isolado. O próprio DSH distingue workflow geral de goals e de um loop fixo de agentes novos (`ralph`).

## Evidência no DSH

- [Grupo Workflow](</Users/yuri/git/other/deepseek-harness/packages/workflow/README.md>)
- [Engine PTC de Workflow](</Users/yuri/git/other/deepseek-harness/packages/workflow/workflow-ptc/README.md>)
- [Subsystem de Workflow](</Users/yuri/git/other/deepseek-harness/docs/subsystems/workflow.md>)
- [Ferramenta Ralph e limites](</Users/yuri/git/other/deepseek-harness/packages/experimental/tool-ralph/README.md>)

## Estado e oportunidade no Opera

O Opera já tem workers com plano persistido, subagentes e execução compartilhada pelo mesmo serviço. Isso cobre delegação e execução de tarefas, mas não necessariamente oferece um mecanismo geral para definir etapas, paralelismo, junção de resultados e saída estruturada. Ver [contrato da CLI](../../../docs/cli-contract.md).

Um workflow explícito pode tornar reproduzíveis tarefas como revisar vários módulos em paralelo e sintetizar achados, mas também pode criar uma segunda linguagem de controle.

## Direção de desenho

- Começar por um catálogo pequeno de workflows tipados/fixos ou uma API sobre ferramentas já existentes; evitar DSL geral antes de haver consumidores.
- Validar entradas/saídas estruturadas, limites de filhos, timeout, tokens, cancelamento e lifecycle de todos os children.
- Propagar policy e identidade de operação por cada etapa; script não concede autoridade adicional.
- Usar checkpoints/eventos duráveis para retomar sem replay de efeitos externos.
- Separar workflows de automação determinística de instruções livres do modelo; rotas e schemas críticos devem ser controlados pela aplicação.

## Critério de sucesso

Um workflow consegue executar etapas em paralelo e agregá-las com limites configurados, cancelar e limpar todos os filhos e retomar sem repetir efeitos já confirmados.
