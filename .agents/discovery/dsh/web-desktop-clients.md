# Clientes Web e Desktop

**Tipo:** superfície de produto e interação

**Prioridade potencial:** decisão estratégica; investimento alto.

## O que o DSH oferece

O DSH separa Host e Client, oferece Web UI e empacota uma aplicação Electron Desktop. A UI usa APIs tipadas sobre conexão compartilhada; controllers mantêm lógica de sessão/workspace do lado Host. O Web profile e o Desktop profile compõem aplicações diferentes sobre capabilities do harness.

## Evidência no DSH

- [Arquitetura de Web, Desktop e Host](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)
- [Pacote API e controllers](</Users/yuri/git/other/deepseek-harness/packages/api/README.md>)
- [Guia de uso Web](</Users/yuri/git/other/deepseek-harness/docs/user/guide/index.md>)
- [Aplicação Desktop](</Users/yuri/git/other/deepseek-harness/apps/desktop/README.md>)

## Estado e oportunidade no Opera

O Opera é um produto CLI/TUI, com scrollback nativo e fluxo acessível append-only. Não há cliente Web/Desktop no layout atual. Ver [plano arquitetural](../../../docs/refactoring-plan.md) e [DESIGN](../../../DESIGN.md).

Uma interface gráfica poderia melhorar navegação por sessões, leitura de diffs/aprovações, visualização de subagentes e configuração de providers. É uma expansão de produto com custos próprios de transporte, autenticação, atualização, empacotamento e suporte.

## Direção de desenho

- Definir primeiro o modelo Host/Client, trust boundary, autenticação, autorização por workspace e comportamento de desconexão.
- Expor operações do produto por API tipada; não permitir que a UI acesse persistence, provider ou execução concreta.
- Derivar views de eventos/projeções com cursores e snapshots consistentes.
- Manter a TUI como cliente plenamente funcional e acessível, não como modo degradado que depende da Web UI.
- Avaliar se o benefício justifica operar um processo local de longa duração antes de iniciar o frontend.

## Critério de sucesso

Um cliente separado acompanha e controla sessões com autorização por workspace, renderiza aprovações e alterações a partir de estado consistente e reconecta sem perder eventos ou repetir operações.
