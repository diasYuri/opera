# Instruções do repositório Opera

## UI de terminal

- Para construir ou alterar qualquer UI de terminal (TUI), use como stack padrão e aproveite amplamente as capacidades das bibliotecas Bubble Tea, Huh, Lip Gloss, Glamour, Bubbles e Harmonica já usadas ou previstas pelo projeto.
- Antes de criar componentes próprios ou lógica de interação/renderização, verifique se a biblioteca adequada já oferece o comportamento. Use Bubble Tea para o ciclo de mensagens e atualização; Huh para formulários, seletores e prompts; Lip Gloss para layout e estilo; Glamour para Markdown; Bubbles para componentes interativos; Harmonica para movimento e transições.
- Integre as bibliotecas em conjunto quando o fluxo pedir, preservando composição, acessibilidade, redimensionamento, `NO_COLOR` e `--no-motion`. Não substitua uma capacidade disponível por uma implementação paralela sem uma limitação técnica concreta; registre a razão quando houver essa limitação.
- “Usar tudo” significa tirar proveito das capacidades relevantes dessas bibliotecas ao longo da UI, sem adicionar chamadas artificiais ou animações que não sirvam à interação.

## Paradigma de desenvolvimento da UI

- Para qualquer mudança de UI em `internal/ui` — componentes, telas, fluxos, temas ou layout — aplique a skill local [opera-tui-ui](.agents/skills/opera-tui-ui/SKILL.md).
- Use princípios de composição, estado explícito, componentes reutilizáveis e tokens semânticos inspirados no SwiftUI e nas interfaces do iPhone, traduzidos para interação de terminal. A inspiração orienta organização e consistência; não reproduza padrões visuais móveis que prejudiquem teclado, scrollback, acessibilidade ou legibilidade do terminal.
- `DESIGN.md` é a fonte de verdade para a identidade visual atual do Opera. Se uma proposta da skill ou uma referência externa divergir dele, mantenha o sistema existente ou atualize-o deliberadamente como parte da tarefa.
- Prefira componentes pequenos com responsabilidades claras e composição entre eles. Antes de criar abstrações ou renderizadores próprios, confira os componentes das bibliotecas Charm já adotadas e justifique limitações concretas quando não puderem ser usados.
- Preserve atualização eficiente: mantenha `Update` como dono das transições de estado e efeitos via `tea.Cmd`; mantenha `View`/renderers determinísticos, sem I/O nem mutação de estado; limite trabalho de renderização ao conteúdo visível e às dimensões atuais do terminal.
- Trate temas como dados semânticos compartilhados, respeitando `NO_COLOR`, fundo/foreground do terminal, `--no-motion`, redimensionamento e modo acessível. Não introduza cores ou estilos específicos de componente fora do tema sem necessidade documentada.
