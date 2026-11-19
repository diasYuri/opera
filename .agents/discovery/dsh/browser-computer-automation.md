# Automação de navegador e computador

**Tipo:** capability de interação externa

**Prioridade potencial:** média-alta para workflows que exigem sites ou aplicações gráficas.

## O que o DSH oferece

O DSH tem seams separados para browser use e computer use, com providers selecionáveis. Há providers experimentais para Playwright MCP, Chrome DevTools MCP, Stagehand e automação de desktop/CUA. Recursos de browser são associados à Session; estado de página não é reconstruído só a partir do transcript.

## Evidência no DSH

- [Pacote browser-use](</Users/yuri/git/other/deepseek-harness/packages/browser-use/README.md>)
- [Provider Playwright MCP](</Users/yuri/git/other/deepseek-harness/packages/experimental/browser-use-playwright-mcp/README.md>)
- [Pacote computer-use](</Users/yuri/git/other/deepseek-harness/packages/computer-use/README.md>)
- [CUA experimental](</Users/yuri/git/other/deepseek-harness/packages/experimental/cua/README.md>)

## Estado e oportunidade no Opera

O inventário atual de ferramentas do Opera inclui filesystem, processos, Git, busca, LSP, tasks, MCP e WASM; browser/computer use não aparece como provider nativo. MCP pode fornecer ferramentas semelhantes, mas não necessariamente a seleção, ownership e UX específica de uma sessão de navegador. Ver [ferramentas do Opera](../../../docs/tools.md).

Isso pode ampliar tarefas de coding que dependem de reproduzir bugs, validar interfaces ou consultar sistemas web autenticados.

## Direção de desenho

- Integrar como capability/provider explícito sob o runtime Y, com recursos e cleanup vinculados ao owner da execução.
- Separar sessão do navegador do histórico durável: o transcript registra ações/resultados; cookies, páginas e conexões são estado vivo com ciclo de vida próprio.
- Aprovação deve ocorrer antes do efeito externo; cancelamento não implica que clique/navegação já enviados foram revertidos.
- Limitar origens, downloads, screenshots, credenciais e persistência de estado conforme política de workspace e provider.
- Preferir provider mantido e testado a implementar protocolo de browser no Opera.

## Critério de sucesso

O usuário pode observar quais sessões e páginas estão ativas, aprovar efeitos relevantes, cancelar com resultado honesto e garantir que fechamento/resume não reutiliza estado de navegador sem consentimento explícito.
