# Distribuição e catálogo de extensões

**Tipo:** ecossistema e gestão de capabilities

**Prioridade potencial:** alta apenas se distribuição de extensões de terceiros for um objetivo do Opera.

## O que o DSH oferece

O DSH separa bundles instaláveis, perfis que os compõem, plugin manager e índices de descoberta. Um índice publica metadados e versões; a instalação mostra a origem e passa pelo gerenciador de pacotes do perfil. A composição pode ser atualizada/recarregada quando o perfil suporta HMR.

## Evidência no DSH

- [Arquitetura de profiles e bundles](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)
- [Plugin Manager](</Users/yuri/git/other/deepseek-harness/packages/boot/plugin-manager/README.md>)
- [Catálogo de plugins](</Users/yuri/git/other/deepseek-harness/docs/user/guide/plugin-market.md>)
- [Bundles base e de aplicação](</Users/yuri/git/other/deepseek-harness/packages/bundle/README.md>)

## Estado e oportunidade no Opera

O Opera aceita tool packs compilados, MCP, módulos WASM consentidos e skills locais. Ele não tem atualmente um fluxo equivalente para descobrir, instalar, atualizar e remover pacotes externos. A gestão de extensões já exige consentimento, grants, identidade de conteúdo e carregamento explícito; ver [documentação de extensões](../../../docs/extensions.md).

Um catálogo poderia facilitar compartilhamento de ferramentas, integrações e skills, desde que não transforme metadados de marketplace em confiança implícita.

## Direção de desenho

- Começar com catálogo de metadados e instalação versionada/pinada; descoberta não deve executar código.
- Exibir origem, versão, digest, permissões e dependências antes de ativar.
- Preservar consentimento por digest/configuração/grants e invalidá-lo quando qualquer um mudar.
- Não adotar execução de plugins nativos no processo como mecanismo de isolamento. Manter WASM/processo mediado para código não confiável.
- Fazer remoção/reinstalação respeitar o lifecycle público de componentes Y e não substituir silenciosamente gerações em uso.

## Critério de sucesso

Um usuário consegue avaliar origem e grants, instalar uma versão identificada, ativá-la com consentimento explícito e removê-la com cleanup completo; falha de ativação não deixa handlers, processos ou consentimento obsoleto.
