# ADR 011: Distribuição de bundles externos

- Status: proposta para avaliação. Implementação e ativação automática aguardam
  revisão de segurança e aceitação do contrato.

## Contexto e avaliação

O catálogo de composição atual seleciona código compilado confiável. O SDK Y
v1.4.0 fornece escopos, componentes, reconciliação de extensões e runtime views.
Esses contratos são adequados para ativação e teardown já autorizados. Um
sistema de distribuição precisa também tratar origem, aquisição de artefatos,
dependências, confiança, consentimento, recuperação e remoção em disco.

A primeira modalidade viável é um bundle WASM isolado com manifesto versionado.
O manifesto deve declarar ID de namespace, versão, digest de conteúdo,
versões/ABI do host, serviços requeridos, entrypoint, limites e grants pedidos.
Separar confiança no publicador de consentimento para efeitos no workspace:
a assinatura verifica a procedência, o digest fixa os bytes e a autorização
por digest vincula módulo, configuração efetiva, grants e escopo de execução.
Manter chaves confiáveis e revogações em configuração do usuário e verificar
assinaturas e digests antes de extrair ou invocar qualquer artefato.

Um processo externo só é opção para código não confiável quando isolado por
um mecanismo do sistema operacional e com API mediada, limites de recursos
e identidades próprias. O MCP stdio atual executa comandos configurados
pelo usuário e, por si só, não impõe esse isolamento. Extensões MCP remotas
continuam exigindo consentimento pelo endpoint e suas capacidades. Plugins
Go arbitrários no processo teriam os privilégios do Opera e política e
capabilities seriam insuficientes para conter seus efeitos.

## Contratos necessários para um projeto futuro

- **Resolução e compatibilidade.** Fixar versões transitivas, digests e
  origem em um lockfile. Validar ABI, OS/arquitetura quando aplicável,
  constraints de serviço, ciclos, colisões de ID, escopo e grants antes da
  ativação. Dependências instaladas não herdam grants nem autorização.
- **Instalação.** Adquirir em staging temporário com limites de tamanho e
  extração confinada, validar todos os bytes e gravar a instalação
  atomicamente em armazenamento do usuário. Uma instalação começa inativa.
  Consentimento vira operação explícita, persistida por identidade e digest.
- **Atualização e rollback.** Adquirir e verificar uma nova versão
  separadamente. Preservar versão e dados anteriores até concluir a ativação.
  Exigir novo consentimento para conteúdo ou autoridade alterados. Falhas
  antes do commit mantêm a seleção anterior. Um rollback depende de versão
  compatível, consentimento ainda válido e dados legíveis. Emitir novamente
  efeitos externos ou reverter dados duráveis exige protocolo próprio.
- **Remoção e GC.** Desativar no ponto suportado, drenar views e remover
  apenas os artefatos sem referência. Preservar checkpoints, transcrições e
  dados do usuário. Definir revogação e auditoria de consentimentos.
- **Recuperação.** Testar atualizações interrompidas, instalações
  concorrentes, manifesto malicioso, traversal, symlinks, downgrade, chaves
  revogadas, troca de bytes, timeout, troca com views ativas e aprovação
  pendente em outro processo. Os diagnósticos podem expor estado e IDs
  validados. Configuração efetiva e credenciais permanecem privadas.

## Próxima decisão

Tratar distribuição como projeto separado. Antes de aceitá-lo, definir
formato e confiança do manifesto, origem das chaves, persistência do lockfile e
consentimentos, threat model e revisão de segurança. Começar por WASM com
grants mediados. Planejar ativação em escopo novo como caminho padrão e
reconciliação apenas nos casos publicados e testados pelo Host do Y.
