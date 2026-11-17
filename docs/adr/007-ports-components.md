# ADR 007 — Portas da aplicação e composição aberta

Status: aceita; execução rastreada no plano de refatoração.

## Contexto

O harness já usa Y público, mas mistura factories de providers, storage e hooks.
App ainda conhece implementações de persistência. Descoberta de providers e
validação repetem enumerações. Isso dificulta substituir capacidades sem alterar
fluxos centrais e torna ownership implícito.

## Decisão

Aplicar portas/adaptadores com interfaces pequenas no consumidor e reutilizar
contratos públicos do Y. O core do agente continua exclusivamente no SDK. App
coordena regras do produto; UI/CLI traduzem entrada e saída; harness compõe
componentes declarados no runtime.Harness. CLI conserva a função de bootstrap.

Capacidades variáveis entram por catálogo explícito de factories/descritores,
sem registry global, init ou switch de providers no core. Implementações nativas
são componentes de composição e não precisam de sandbox ou carregamento dinâmico.
MCP/memória/WASM continuam no Host público existente. Catálogo não é runtime.

Storage/contexto/skills/runner são recebidos por portas onde há efeito ou
substituição necessária. Recursos concretos são abertos/fechados na composição.
Componentes declaram dependências com keys compartilhadas e scopes possuem
recursos. Reconfiguração/gerações pertencem ao Y; defaults do produto ao Opera.

## Consequências

Podemos testar app sem driver/provider concreto e instalar capacidade sem alterar
loop ou apresentação. Interfaces exigem manutenção de contratos, portanto não
extraímos cada helper nem criamos framework hexagonal genérico. Não prometemos
hot reload universal: composição estática e Host dinâmico têm contratos distintos.
Não há mudança incidental em persistência, CLI ou identidade de aprovação.

Ver `../refactoring-plan.md` para etapas/aceite e `../tui-development.md` para
composição declarativa da apresentação, preservando ADR 005 e DESIGN.md.
