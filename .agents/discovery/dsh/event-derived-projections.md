# Projeções derivadas de eventos duráveis

**Tipo:** padrão de design de estado e leitura
**Aplicabilidade:** alta para qualquer estado de produto que precise de replay, UI ou leitura eficiente.

## Padrão no DSH

O Session log é a fonte de fatos duráveis. Projeções dobram eventos comprometidos em estado atual para o Host ou para views do Client. Cada domínio registra sua projeção; carriers leem snapshots consistentes e podem usar checkpoints para evitar replay integral em leituras frias.

## Evidência no DSH

- [Session projection](</Users/yuri/git/other/deepseek-harness/packages/session/session-projection/README.md>)
- [Arquitetura de session log e projeções](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)
- [Agent Team projection e journal](</Users/yuri/git/other/deepseek-harness/packages/team/agent-team/README.md>)

## Aplicação possível no Opera

O Opera já persiste checkpoints, eventos, operações e decisões para recuperar execução. Essa abordagem é pertinente para novas views — por exemplo, board de agentes ou estado de automação — que não devem ser armazenadas como uma segunda verdade mutável desligada do histórico durável. Ver [operações e recuperação](../../../docs/operations.md).

## Regras úteis

- Definir qual evento é a fonte de verdade e qual estrutura é uma view descartável/derivada.
- Versionar a semântica da projeção e comprovar equivalência entre restore de checkpoint mais sufixo e replay completo.
- Distinguir cursor de evento de offset de leitura; snapshots precisam declarar até qual evento estão atualizados.
- Projeções de cliente devem publicar apenas os dados necessários e não expor estado host-only.
- Falha de fold deve ser observável; não substituir silenciosamente a última view válida por estado parcial.

## Limite

Não converter cada struct em event sourcing. O padrão compensa quando replay, auditoria ou múltiplos consumidores de leitura justificam a complexidade do formato durável e sua evolução.
