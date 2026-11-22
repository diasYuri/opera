# ADR 013 — Backstage da sessão e compactação por camadas

Status aprovado.

**Backstage** é o gerenciador de trabalho em segundo plano do Opera. A
compactação de contexto é sua primeira capacidade. O Y continua responsável
pela execução do agente, pelo transcript, pelos checkpoints e pelas operações.

## Admissão e medição

O gerenciador pertence ao serviço de execução e drena seus trabalhos antes
do provider e dos stores. Ele usa até dois workers por composição. Cada sessão
principal tem espaço para uma compactação e para a observação concluída mais
recente. Requisições acima do teto avançam na fila. Recursos para uma
requisição viável são reservados antes de enfileirar trabalho opcional.
Cancelamento e encerramento propagam o cancelamento aos trabalhos pendentes.

A requisição montada pelo Y e o checkpoint concluído passam pelo mesmo
medidor. A avaliação final reaproveita as instruções, ferramentas e resolução
de mídia da última requisição. Ela mede de novo depois de uma compactação
do turno que ainda estava em curso. A estimativa local usa
`providers.EstimateTokens`. A janela vem dos metadados locais do modelo,
limitados por `limits.context_tokens`. Esse limite também serve de referência
quando o catálogo desconhece a janela. A compactação automática começa aos
60% da janela operacional e pode ser exigida antes de enviar uma requisição
acima do teto.

Cada sessão principal tem uma thread de inferência de compactação. Subagentes
podem compactar seus próprios transcripts antes de uma nova inferência. Cada
subagente usa uma projeção e uma thread de compactação privativas. As chamadas
de sumarização são contabilizadas no orçamento de trabalho.

A finalização durável do run grava metadados, fronteira e índice antes de
entregar o trabalho final ao Backstage. A execução headless pode aguardar a
publicação do resumo com o prazo próprio da compactação. O chat pode entregar
o resultado enquanto esse trabalho termina em segundo plano.

## Camadas e linhagem

- **L1 — objetivo atual.** Preserva a tarefa, as restrições e os critérios de
  aceite. A camada acompanha a mudança de objetivo.
- **L2 — trabalho e evidência.** Condensa chamadas e respostas em achados,
  decisões, pendências e incertezas. Preserva pares de chamadas e respostas
  nas fronteiras da projeção.
- **L3 — resultados.** Mantém as entregas e decisões concluídas que ainda
  importam para a conversa.

A compactação automática aplica somente projeções cujo prefixo corresponde
ao transcript. Durante um run, a projeção fica em um registro privado. Ao
concluir, o run pode atualizar o contexto compartilhado se for a ponta da
sessão e se a versão de conteúdo que serviu de base ainda for compatível.
Continuações concorrentes podem concluir, e a comparação de versão decide
qual cadeia avança a ponta. Um checkpoint anterior pode iniciar uma nova
sessão com `sessions branch`. Para sessões anteriores a esse registro, a
migração escolhe o checkpoint concluído mais recente do Y, mesmo que o
usuário tenha reaberto um run mais antigo depois.

O registro versionado da sessão contém Markdown, projeção e identidade da
ponta atual. O agente principal e seus descendentes recebem o contexto da
sessão raiz e podem registrar fatos via `session_context_note`. O teto
conservador de 5.000 bytes UTF-8 limita o Markdown a 5.000 tokens nas
codificações por bytes. O arquivo privado
`$OPERA_DATA_DIR/sessions/<hash>/context.md` materializa o registro canônico.
Falhas de materialização são registradas e uma execução posterior pode
recriar o arquivo a partir do banco.

## Compactação manual e recuperação

`/compact` e `opera compact <run-id>` operam no checkpoint concluído da ponta
da sessão. A operação grava um arquivo `prepared` com o original e a
compactação proposta. A publicação canônica registra o identificador do
arquivo e aceita as duas representações equivalentes do prefixo, antes e
depois da reescrita do checkpoint. O arquivo avança para `published` e, após
a reescrita do Y, para `committed`.

Uma repetição identifica a última etapa confirmada pelo registro canônico e
pelo checkpoint do Y. Ela pode concluir a reescrita ou finalizar o arquivo
usando o mesmo resumo. A materialização do Markdown ocorre depois do commit.
O resultado original do run permanece disponível para replay. Checkpoints
anteriores à ferramenta de notas mantêm a geração antiga se as demais
ferramentas e extensões coincidirem.
