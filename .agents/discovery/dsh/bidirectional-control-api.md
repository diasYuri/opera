# API de controle bidirecional para sessões

**Tipo:** recurso de integração e automação

**Prioridade potencial:** alta quando houver demanda de IDEs, automação ou clientes externos.

## O que o DSH oferece

O DSH expõe agentes persistentes por SDK/JSON-RPC e por ACP. Clientes podem criar ou retomar sessões, enviar prompts, receber atualizações e responder a pedidos de permissão; o transporte mantém o processo vivo para controle interativo. O ACP é voltado à automação, enquanto o SDK JSON-RPC oferece uma API própria do DSH.

## Evidência no DSH

- [Pacotes SDK](</Users/yuri/git/other/deepseek-harness/packages/sdk/README.md>)
- [Servidor SDK JSON-RPC](</Users/yuri/git/other/deepseek-harness/packages/sdk/server/README.md>)
- [Pacote ACP](</Users/yuri/git/other/deepseek-harness/packages/acp/README.md>)
- [Arquitetura e perfis de aplicação](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)

## Estado e oportunidade no Opera

O Opera já tem execução por `run`/`resume`, saída JSONL com IDs e sequência durável, e um SDK público Y subjacente. O contrato atual não oferece uma sessão de chat JSONL bidirecional nem um servidor Opera que permita a um cliente manter a sessão e controlar prompts, cancelamento e aprovações durante a execução. Ver [contrato da CLI](../../../docs/cli-contract.md).

Uma API de controle ao vivo poderia integrar o Opera a editores, interfaces próprias e automações sem depender de interpretar a saída de um processo one-shot.

## Direção de desenho

- Reutilizar `execution.Service`, checkpoints, IDs, eventos, política e fluxo de aprovações já existentes.
- Tratar o protocolo como adapter de transporte; não criar outro runner nem uma segunda autoridade de sessão.
- Definir correlação, replay, backpressure, cancelamento, fechamento e recuperação após desconexão antes de expor o protocolo.
- Manter o comportamento de foreground da CLI independente; um servidor persistente seria um modo/processo explícito com lifecycle e persistência próprios.

## Critério de sucesso

Um cliente consegue iniciar ou retomar uma execução, receber eventos ordenados, cancelar, responder a uma aprovação e reconectar sem duplicar efeitos nem contornar as garantias atuais de recuperação.
