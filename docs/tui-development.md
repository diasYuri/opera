# Paradigma de desenvolvimento da TUI

## Estado descreve a interface

A inspiração em SwiftUI é composição declarativa, identidade estável, fluxo de dados
unidirecional e ownership explícito. A implementação é Go idiomático sobre Charm.
`View = render(state, dimensions, theme)`; mensagens representam intenções/resultados;
`Update` aplica transições; `tea.Cmd` executa efeitos e retorna novas mensagens.

| Conceito SwiftUI | Tradução para Opera |
| --- | --- |
| View/body | Função pura de composição ou View de submodelo |
| State | Campos privados no menor model que possui a interação |
| Binding | Intenção/mensagem tratada pelo owner; sem ponteiros mutáveis cruzados |
| Environment | Dependências, dimensões, tema e preferências explicitamente injetados |
| Observable model | Snapshot/ação/evento da aplicação, sem acesso a driver/provider |
| Task/onChange | Cmd cancelável emitido por Init/Update e resultado identificado |
| View identity | ID estável de mensagem, run, item e geração; nunca índice acidental |
| ViewModifier | Estilo/helper semântico Lip Gloss; sem DSL paralela |

## Composição e responsabilidades

O shell coordena foco, navegação, ciclo do terminal e encaminhamento de mensagens.
Composer possui edição/histórico; seleção possui lista e cursor; painel possui
viewport e conteúdo; atividade possui ticks/geração; conversação possui projeção e
identidade das entradas. Use função para peça sem estado e submodelo para estado
independente. Não crie interface de componente obrigatória para todo render helper.
O shell não duplica estado já possuído pelo componente. Dados derivados baratos são
calculados; cache caro declara invalidação por conteúdo, largura e tema.

Componentes recebem somente dependências necessárias e emitem intenções tipadas.
A execução de ferramenta, persistência, catálogo remoto e clipboard/processos são
efeitos por portas, nunca trabalho de View. Um componente de apresentação não recebe
`*app.Service` se uma interface menor basta. Controles não conhecem harness.

## Stack e interação

Bubble Tea: ciclo Update/Msg/Cmd. Bubbles: textarea, list, viewport, keymap e spinner.
Huh: formulários, seletores e prompts. Lip Gloss: layout e tokens semânticos.
Glamour: Markdown com renderizador reutilizado por largura/tema. Harmonica: movimento
informativo durante atividade. Avalie estas APIs antes de implementar um controle.
Exceções precisam de limitação concreta documentada.

As fontes de TUI ficam em `internal/ui/tui`. DESIGN.md prevalece para identidade visual: terminal fornece fonte, foreground e
background; coluna única, composer com regras finas, histórico em viewport independente e composer fixo no
rodapé, usando alternate screen no chat interativo. O caminho acessível continua
append-only. Não copiar cartões ou navegação por
toque do iPhone. Preservar teclado e restaurar foco ao fechar temporários.

## Efeitos, concorrência e acessibilidade

Uma operação assíncrona recebe contexto do fluxo; cancelamento e encerramento têm
owner. Inclua ID/geração nos resultados que possam chegar após troca de run ou tela.
Descarte resultados antigos em Update. Ticks existem somente durante atividade;
no-motion e estado idle não podem manter redesenho periódico ou cursor piscando.

Layout usa colunas ANSI/Unicode, não bytes. Renderize conteúdo visível, adapte hints,
wrap e clipping em terminais pequenos. Tokens semânticos vêm de `internal/ui/tui/theme.go`. NO_COLOR
é presença da variável, mesmo vazia. Estados possuem texto além de cor. O caminho
acessível é append-only, sem raw-mode, cursor movement ou animação obrigatória.
Contribuições de extensions são dados limitados e sanitizados, nunca renderizadores
arbitrários com acesso ao runtime.

## Validação de uma mudança

Testar transições e efeitos observáveis, foco/cancelamento, stale messages, resize
estreito/baixo, Unicode/ANSI, temas e NO_COLOR vazio, no-motion e acessibilidade.
View repetida sobre o mesmo estado não muda o modelo nem produz efeitos.
Preservar testes PTY/ConPTY existentes; não confundir build cruzado com teste nativo.
Atualizar DESIGN deliberadamente quando a tarefa mudar o sistema visual.
