# Sandbox aplicado pelo sistema operacional

**Tipo:** segurança de execução

**Prioridade potencial:** alta como investigação, pois reduz a distância entre policy declarada e isolamento efetivo.

## O que o Codex oferece

O Codex transforma uma `SandboxPolicy` em restrições executadas por mecanismos do sistema: Seatbelt no macOS, Bubblewrap/namespaces no Linux e backends próprios no Windows. A camada de sandbox controla raízes de leitura/escrita e política de rede; decisões de aprovação e regras de comando continuam sendo camadas separadas. O repositório documenta limitações e falha fechada quando um backend não consegue aplicar uma policy sem enfraquecê-la.

## Evidência no Codex

- [Matriz de sandbox por plataforma](</Users/yuri/git/other/codex/codex-rs/core/README.md>)
- [Abstração de sandbox](</Users/yuri/git/other/codex/codex-rs/sandboxing/src/lib.rs>)
- [Bubblewrap/Linux](</Users/yuri/git/other/codex/codex-rs/sandboxing/src/bwrap.rs>)
- [Seatbelt/macOS](</Users/yuri/git/other/codex/codex-rs/sandboxing/src/seatbelt.rs>)
- [Windows](</Users/yuri/git/other/codex/codex-rs/sandboxing/src/windows.rs>)

## Estado e oportunidade no Opera

O Opera documenta que sua policy não é um sandbox de sistema operacional; `run_command` e subprocessos continuam dependendo de policy, limites e implementação do SDK Y. Ver [ferramentas e limites](../../../docs/tools.md) e [operações](../../../docs/operations.md).

Um adapter de sandbox pode oferecer defesa em profundidade para ferramentas que executam processos, particularmente em modo `ask` ou `trusted`. Isso não substitui aprovação nem autorização por operação.

## Direção de desenho

- Investigar primeiro as APIs públicas e suportadas do SDK Y; não duplicar executor ou usar integração privada.
- Modelar capability real por plataforma/backend e recusar policy que o backend não consegue garantir.
- Manter autorização, approval e sandbox como decisões distintas: permissão de iniciar processo não significa permissão de acesso irrestrito.
- Validar raízes canônicas, symlinks, `.git`, arquivos de configuração e acesso à rede com fixtures por plataforma.
- Deixar explícito quando um ambiente cai para execução sem sandbox; não usar fallback silencioso.

## Critério de sucesso

Um processo filho e seus descendentes não conseguem ler, escrever ou acessar rede além da policy efetivamente aplicada; backend indisponível ou incapaz de satisfazer a policy falha com erro contextual.
