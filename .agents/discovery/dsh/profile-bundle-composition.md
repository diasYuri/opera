# Composição em perfis, bundles e overlays

**Tipo:** padrão de composição configurável
**Aplicabilidade:** média se crescer o número de produtos, distribuições ou presets do Opera.

## Padrão no DSH

Perfis selecionam bundles em ordem e aplicam patches/overlays posteriores. Bundles definem linhas de configuração e pacotes, mas o conteúdo continua patchável por camadas superiores. O DSH pode inspecionar a árvore efetiva com `--dump-config`.

## Evidência no DSH

- [Perfis, bundles e precedência](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)
- [Bundles](</Users/yuri/git/other/deepseek-harness/packages/bundle/README.md>)
- [Configuração do CLI](</Users/yuri/git/other/deepseek-harness/apps/cli/composition.md>)

## Aplicação no Opera

O Opera já tem configuração em camadas e catálogo explícito de tool packs/providers, mas não organiza distribuições como perfis de plugins. Perfis poderiam fazer sentido se surgirem composições suportadas realmente diferentes — por exemplo, local restrito, automação ou integração com editor. Ver [configuração](../../../docs/configuration.md) e [extensões](../../../docs/extensions.md).

## Regras úteis

- Fornecer inspeção da composição resolvida e da origem/proveniência de cada valor.
- Tornar precedência determinística e rejeitar IDs desconhecidos, colisões e dependências ausentes.
- Manter configuração de confiança e credenciais sob autoridade do usuário; um overlay do workspace não deve elevar policy.
- Evitar dois mecanismos de seleção para a mesma capability.
- Permitir um profile mínimo sem capacidades opcionais e falhar cedo quando faltar uma capability obrigatória.

## Limite

Não introduzir bundles apenas para renomear conjuntos internos de flags. A abstração vale quando existem composições nomeadas mantidas e consumidores que precisam selecioná-las ou inspecioná-las.
