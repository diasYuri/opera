# Efeitos reversíveis e ownership de lifecycle

**Tipo:** padrão de composição e cleanup
**Aplicabilidade:** alta para capabilities com registrations ou recursos dinâmicos.

## Padrão no DSH

No Cordis, registrar ferramentas, listeners, providers e seções de prompt é um efeito associado ao plugin proprietário. O disposer desfaz a contribuição ao descarregar o plugin. Recursos de runtime têm owner e cleanup ligados ao ciclo de vida do serviço.

## Evidência no DSH

- [Cordis primer: effects e dispatch](</Users/yuri/git/other/deepseek-harness/docs/cordis-primer.md>)
- [Arquitetura e unload de plugins](</Users/yuri/git/other/deepseek-harness/docs/architecture.md>)
- [Plugin manager: reload, falhas e publication](</Users/yuri/git/other/deepseek-harness/packages/boot/plugin-manager/README.md>)

## Aplicação no Opera

O Opera já usa lifecycle e scopes públicos de Y; suas instruções requerem que `Start` adquira recursos e registre ownership imediatamente, e que falha parcial feche recursos. A ideia útil é associar cada registration e recurso ao owner correto, não copiar a API `ctx.effect()` do Cordis. Ver [plano arquitetural](../../../docs/refactoring-plan.md).

## Regras úteis

- Factory descreve composição; não abre conexão ou inicia worker.
- Startup registra ownership logo após cada aquisição para rollback parcial.
- Cleanup é idempotente, em ordem inversa de dependência e tenta fechar todos os recursos.
- Reload usa safe points/runtime views; não troca handlers que uma operação em andamento ainda utiliza.
- Distinguir cleanup de compensação: fechar recurso não desfaz arquivo escrito ou chamada externa.

## Limite

Nem todo efeito é reversível. Publicar uma mensagem, escrever um arquivo ou acionar um endpoint pode ser durável e exigir auditoria/reconciliação em vez de um disposer fictício.
