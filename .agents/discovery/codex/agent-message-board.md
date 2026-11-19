# Quadro de mensagens entre agentes

**Tipo:** colaboração entre agentes

**Prioridade potencial:** média-alta para delegação que precise compartilhar decisões e anúncios além de resultados finais.

## O que o Codex oferece

O Codex tem ferramentas de criação e gestão de agentes filhos e extensões de message board que permitem compartilhar canais e mensagens entre runtimes. O host pode conectar runtimes com permissões diferentes, inclusive somente leitura. A documentação alerta que essas permissões da extensão não são ACLs de banco de dados e que escritores locais confiáveis continuam fora desse controle.

## Evidência no Codex

- [Host do Agent Message Board](</Users/yuri/git/other/codex/codex-rs/agent-message-board-host/README.md>)
- [Client do board](</Users/yuri/git/other/codex/codex-rs/agent-message-board-client/>)
- [Tipos de mensagens e permissões](</Users/yuri/git/other/codex/codex-rs/message-board/src/permissions.rs>)
- [Ferramentas e roteamento de agentes](</Users/yuri/git/other/codex/codex-rs/core/src/tools/router.rs>)

## Estado e oportunidade no Opera

O Opera mantém filhos, status, tarefas e mensagens do lifecycle de execução, mas o contrato voltado ao usuário não apresenta um canal persistente de discussão entre workers. Um board pode tornar decisões e bloqueios compartilhados mais fáceis de acompanhar, especialmente quando há também um quadro de tarefas.

## Direção de desenho

- Separar mensagens de agente da verdade sobre tarefas: mensagens não concluem trabalho nem autorizam efeitos.
- Escopar canais a uma árvore/run e validar sender/recipient usando a identidade Y existente.
- Definir retenção, limites de tamanho, replay após resume e comportamento de membros offline.
- Não tratar controle de UI/extensão como ACL do storage; qualquer boundary de confiança precisa ser aplicada no host.
- Evitar acordar agentes ociosos automaticamente sem uma política de scheduling explícita.

## Critério de sucesso

Mensagens têm autoria e sequência verificáveis, não cruzam árvores sem permissão, sobrevivem ao ciclo de vida previsto e continuam sem poder modificar tasks ou conceder capacidade por si mesmas.
