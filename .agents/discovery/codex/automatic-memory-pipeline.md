# Memória automática em duas fases

**Tipo:** recuperação de conhecimento entre sessões

**Prioridade potencial:** média; requer consentimento e um modelo claro de revisão/remoção.

## O que o Codex oferece

O pipeline de memória do Codex seleciona rollouts anteriores, extrai registros estruturados em jobs limitados e depois consolida os resultados em artefatos locais. A extração e a consolidação são fases distintas, com leases, limites de concorrência, retries e um baseline/diff da área de memória. A documentação descreve execução assíncrona quando uma sessão raiz inicia e restrições de elegibilidade.

## Evidência no Codex

- [Pipeline de memória](</Users/yuri/git/other/codex/codex-rs/memories/README.md>)
- [Leitura de memória](</Users/yuri/git/other/codex/codex-rs/memories/read/src/lib.rs>)
- [Escrita e consolidação](</Users/yuri/git/other/codex/codex-rs/memories/write/src/lib.rs>)
- [Orquestração no core](</Users/yuri/git/other/codex/codex-rs/core/src/memories/>)

## Estado e oportunidade no Opera

O Opera já tem memória local por workspace, mas gravação e esquecimento são explícitos; extração automática está desabilitada por decisão de produto. Ver [documentação de extensões e memória](../../../docs/extensions.md).

Uma pipeline opt-in poderia poupar ao usuário o trabalho de salvar fatos úteis manualmente. Também aumenta o risco de guardar informação incorreta, sensível ou obsoleta; a arquitetura atual do Opera favorece consentimento e controle humano.

## Direção de desenho

- Tratar como modo opt-in separado, sem alterar o comportamento padrão de memória explícita.
- Usar fontes elegíveis, janela de retenção, limites e leases para não processar rollouts ativos nem duplicar jobs concorrentes.
- Aplicar redação de segredos antes da persistência e manter proveniência para cada fato extraído.
- Produzir diff revisável e permitir editar/apagar itens; recuperação deve ser contexto, nunca instrução de sistema com autoridade.
- Não iniciar background worker destacado na CLI sem definir processo proprietário, shutdown, retry e readiness.

## Critério de sucesso

Com consentimento ativo, a pipeline gera uma proposta de memória com proveniência e diff revisável, elimina segredos conhecidos e permite corrigir/remover fatos sem afetar transcript ou checkpoints originais.
