# Policy traduzida em enforcement do sistema operacional

**Tipo:** padrão de segurança em camadas
**Aplicabilidade:** alta quando há execução de processos ou adapters com efeitos locais.

## Padrão no Codex

O Codex mantém uma policy semântica e a traduz para backends específicos de sistema operacional. A policy informa raízes e permissões; o helper de sandbox aplica as restrições no processo filho. A lógica de aprovação/exec policy não é tratada como substituta do sandbox. Quando uma plataforma não consegue representar com segurança a policy solicitada, o comportamento documentado é rejeitar em vez de reduzir silenciosamente o isolamento.

## Evidência no Codex

- [Policy de permissões](</Users/yuri/git/other/codex/codex-rs/protocol/src/permissions.rs>)
- [Transforms de policy](</Users/yuri/git/other/codex/codex-rs/sandboxing/src/policy_transforms.rs>)
- [Gerenciador de sandbox](</Users/yuri/git/other/codex/codex-rs/sandboxing/src/manager.rs>)
- [Exec policy](</Users/yuri/git/other/codex/codex-rs/execpolicy/README.md>)

## Aplicação no Opera

O Opera separa policy `read-only`/`ask`/`trusted`, approval por operação e adapters de execução; a documentação deixa claro que policy não é sandbox de sistema operacional. Essa separação é correta e deve permanecer mesmo que se investigue enforcement OS-level. Ver [operações](../../../docs/operations.md) e [ferramentas](../../../docs/tools.md).

## Regras úteis

- Autorização decide se o efeito é permitido; sandbox limita o que um processo permitido consegue alcançar.
- Resolver paths/symlinks e redes permitidas antes de materializar a policy nativa.
- Testar a policy efetivamente aplicada, não apenas o objeto de configuração.
- Emitir diagnóstico quando backend ou versão não suporta a garantia solicitada.
- Não inferir que um comando “read-only” é seguro por nome ou classificação superficial.

## Limite

As garantias do Codex dependem de helpers e recursos do sistema operacional. Uma feature do Opera deve reportar limites de cada plataforma e não anunciar confinamento que o SDK público Y/host não consegue garantir.
