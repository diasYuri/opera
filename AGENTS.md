# Instruções do repositório Opera

## UI de terminal

- Para construir ou alterar qualquer UI de terminal (TUI), use como stack padrão e aproveite amplamente as capacidades das bibliotecas Bubble Tea, Huh, Lip Gloss, Glamour, Bubbles e Harmonica já usadas ou previstas pelo projeto.
- Antes de criar componentes próprios ou lógica de interação/renderização, verifique se a biblioteca adequada já oferece o comportamento. Use Bubble Tea para o ciclo de mensagens e atualização; Huh para formulários, seletores e prompts; Lip Gloss para layout e estilo; Glamour para Markdown; Bubbles para componentes interativos; Harmonica para movimento e transições.
- Integre as bibliotecas em conjunto quando o fluxo pedir, preservando composição, acessibilidade, redimensionamento, `NO_COLOR` e `--no-motion`. Não substitua uma capacidade disponível por uma implementação paralela sem uma limitação técnica concreta; registre a razão quando houver essa limitação.
- “Usar tudo” significa tirar proveito das capacidades relevantes dessas bibliotecas ao longo da UI, sem adicionar chamadas artificiais ou animações que não sirvam à interação.
