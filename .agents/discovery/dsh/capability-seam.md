# Seam de capability: definição, provider e consumidor

**Tipo:** padrão de arquitetura de extensibilidade
**Aplicabilidade:** média-alta ao introduzir uma capacidade substituível.

## Padrão no DSH

O DSH define uma capability como três papéis: definição do serviço, provider que implementa o serviço e consumidor que usa a capability, frequentemente uma ferramenta apresentada ao modelo. A separação impede chamar de “seam” uma interface sem provider ou um provider sem consumidor real.

## Evidência no DSH

- [Capability seams](</Users/yuri/git/other/deepseek-harness/docs/capability-seams.md>)
- [Mapa de capabilities e pontos de extensão](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)
- [Princípios de composição Cordis](</Users/yuri/git/other/deepseek-harness/docs/cordis-primer.md>)

## Aplicação no Opera

O princípio é compatível com a arquitetura Opera de componentes Y e tool packs: se algo for selecionável/substituível, é preciso identificar quem define o contrato, quem fornece a implementação e quem a consome. Isso não implica criar uma interface para cada chamada interna. Ver [refatoração arquitetural](../../../docs/refactoring-plan.md).

## Perguntas de projeto

- Qual é o serviço/contrato público e quem é seu único owner?
- Qual factory/provider registra a implementação e como a composição escolhe a instância?
- Qual consumidor real usa o contrato no caminho padrão?
- Quais capacidades são obrigatórias, opcionais ou substituíveis?
- Como lifecycle, policy, erro e ausência do provider são observáveis?

## Limite

Se não há variação real, efeito externo ou consumidor isolável, chamadas concretas entre módulos coesos podem ser mais claras que uma interface. O padrão ajuda a fechar uma capability completa, não a multiplicar abstrações.
