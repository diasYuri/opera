---
name: opera-tui-ui
description: Desenvolva e reorganize a interface TUI do Opera com composição de componentes, temas consistentes e renderização eficiente, usando princípios de SwiftUI adaptados ao terminal.
metadata:
  short-description: Paradigma de UI TUI do Opera
---

# UI TUI do Opera

Use esta skill ao criar, alterar, revisar ou reorganizar telas, componentes, fluxos de interação, layout e temas em `internal/ui/tui`.

## Direção

Adote da interface SwiftUI/iPhone os princípios de composição declarativa, hierarquia clara, estado explícito, componentes focados e design tokens consistentes. Traduza esses princípios para uma interface de terminal com prioridade para teclado, texto, scrollback nativo, adaptação a dimensões variáveis e acessibilidade. Não imite controles, cartões, navegação por toque ou animações móveis quando não se encaixarem no terminal.

Antes de decidir o desenho, consulte `DESIGN.md` e os componentes já existentes em `internal/ui/tui`. `DESIGN.md` define a direção visual do Opera e prevalece sobre referências genéricas; a implementação em Go e as bibliotecas Charm determinam como compor a interface.

## Modelo mental

- **Tela é composição:** organize cada superfície como uma composição de áreas e componentes com funções nomeadas. Reaproveite elementos visuais e comportamentos quando houver mais de um consumidor ou uma responsabilidade realmente comum; evite abstrações genéricas sem uso concreto.
- **Estado tem proprietário:** mantenha estado de interação no `model`/submodelo responsável. Atualize estado e trate eventos em `Update`; represente efeitos externos como comandos Bubble Tea (`tea.Cmd`) e devolva seus resultados como mensagens.
- **Renderização descreve o estado atual:** `View` e helpers de renderização devem derivar saída do estado e das dimensões disponíveis. Não façam I/O, não iniciem goroutines, não alterem o modelo nem agendem trabalho recorrente.
- **Tema é semântico:** consuma a paleta central de `internal/ui/tui/theme.go` para texto, contexto secundário, acento, informação, aviso, erro, sucesso e borda. Um componente deve expressar intenção sem escolher sua própria paleta. Mantenha texto compreensível sem cor.
- **Adaptação é parte do componente:** passe ou derive largura e altura disponíveis; limite conteúdo, preserve wrap e clipping Unicode/ANSI apropriados e simplifique hints em larguras estreitas. Não suponha tamanho fixo de terminal.

## Organização e componentização

- Mantenha a fronteira entre ciclo de vida e apresentação: Bubble Tea coordena eventos, estado e comandos; Bubbles fornece controles interativos; Huh fornece formulários e seletores; Lip Gloss compõe estilo e layout; Glamour renderiza Markdown; Harmonica é reservado para movimento útil durante atividade.
- Prefira componentes idiomáticos em Go: funções pequenas de renderização para peças sem estado independente; structs/submodelos quando um componente possui estado, eventos ou ciclo de vida próprio. Use interfaces apenas em fronteiras que precisem de substituição ou extensão.
- Separe arquivos por fluxo ou responsabilidade coesa quando isso melhorar navegação e manutenção. Evite mover código apenas para obter uma estrutura rígida de pastas, duplicar estado entre componentes ou criar camadas de wrappers ao redor das APIs Charm.
- Centralize estilos e preferências de tema. Ao adicionar um token, escolha um nome semântico, forneça comportamento coerente para os temas suportados e mantenha a versão sem cor funcional.
- Preserve as fronteiras existentes entre UI e domínio/serviços. Componentes não devem buscar dados diretamente em providers, persistir configurações ou executar ferramentas; solicitem essas ações pelo fluxo de comandos/serviços já usado pela aplicação.

## Desempenho e atualização

- Faça primeiro o desenho simples e legível. Otimize a partir de um caminho quente identificado, medição ou custo evidente; evite cache ou estado derivado duplicado que complique invalidação.
- Não faça trabalho caro a cada frame sem necessidade: evite recriar renderizadores de Markdown, estilos compostos ou grandes listas quando puderem ser preparados na mudança relevante e reutilizados com segurança. Ao introduzir cache, defina claramente a chave de invalidação, especialmente para tema e largura.
- Renderize apenas o conteúdo necessário à janela visível quando houver listas ou histórico extenso; use viewport/lista de Bubbles e limites de altura coerentes com o fluxo.
- Não agende ticks para estado ocioso. Animações e atualizações periódicas devem ter propósito, parar ao concluir/cancelar e respeitar `--no-motion`.
- Ao tratar texto externo, preserve sanitização e tratamento de escapes ANSI já existentes; dimensione por colunas de terminal, não por bytes ou caracteres Unicode presumidos como largura 1.

## Acessibilidade e temas

- Respeite `NO_COLOR` por presença da variável, inclusive vazia, e não dependa apenas de cor para seleção, erro, aprovação ou estado ativo.
- Preserve fundo e foreground fornecidos pelo terminal. Não adicione fundos, bordas decorativas ou cores literais fora do sistema estabelecido em `DESIGN.md`.
- Mantenha alternativas de teclado, foco visível, hints contextuais e caminho de uso acessível. Se um controle não puder ser usado no modo acessível, forneça uma alternativa textual/por teclado compatível com os padrões do projeto.
- Movimento é opcional e informativo; mantenha o estado compreensível sem animação. Evite cursor piscante ou redesenho contínuo em idle.
- Ao compor formulários, use Huh; configure dimensões e tema de acordo com a tela atual e garanta que cancelamento/retorno restaure o foco e contexto corretos.

## Checklist para uma mudança

1. Identifique a tela, seu estado proprietário, as mensagens envolvidas e as dimensões mínimas que precisa suportar.
2. Inspecione o componente Charm apropriado e os tokens existentes antes de implementar uma alternativa própria.
3. Componha a UI a partir das responsabilidades existentes; evite duplicar estado, lógica de interação e estilos.
4. Verifique o fluxo com `NO_COLOR`, `--no-motion`, redimensionamento e modo acessível quando a mudança afetar esses comportamentos.
5. Para qualquer exceção à stack ou ao sistema visual, registre no código ou na documentação a limitação concreta que a motivou.

## Referências do repositório

- `DESIGN.md`: identidade visual, layout, componentes, acessibilidade e limites de movimento.
- `internal/ui/tui/theme.go`: paletas semânticas e estilos de controles reutilizados.
- `internal/ui/tui/layout.go`: ciclo de layout, dimensões, status e movimento.
- `internal/ui/tui/chat.go`: estado principal, transições, comandos e composição dos fluxos.
- `internal/ui/tui/conversation.go`: composição de conteúdo, Markdown e mensagens.
