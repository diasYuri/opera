# ADR 014: Verificação e atualização explícita de versões

- Status: aceito.

## Contexto

Opera precisa informar releases disponíveis e oferecer um caminho verificável
para atualizar a própria instalação. Projetos Go também precisam consultar e
aplicar atualizações de módulos sem expor credenciais do provider nem executar
código do projeto.

## Decisão

- A CLI oferece `opera update check|apply`. A checagem consulta somente releases
  semânticos publicados no repositório oficial `diasYuri/opera`; não há polling,
  daemon nem atualização automática.
- `apply` é uma ação explícita. O adapter baixa o pacote da plataforma, valida
  checksum, paths, tipos de arquivo, tamanho e metadados de versão/plataforma. O
  executável é preparado ao lado do atual e trocado por rename atômico. O fluxo
  é suportado em macOS/Linux, requer permissão de escrita no diretório e não usa
  `sudo`.
- `opera deps check|update` suporta Go modules pela toolchain `go`. Checagem usa
  `go list -m -u -json all`. Atualização usa `go get` e `go mod tidy`; upgrades
  gerais permanecem no major path atual, e `patch` restringe para patches.
  Migrações de major com alterações de código são manuais. `go.mod`/`go.sum` são
  restaurados se qualquer comando da atualização falhar.
- Os comandos não carregam configuração de provider e funcionam sem credenciais.
  Os efeitos de rede aparecem apenas quando o comando de versão é invocado. A
  atualização de dependências baixa metadados e módulos, mas não executa build,
  testes nem scripts do projeto.

## Consequências

As consultas são previsíveis e não alteram o workspace. A instalação local do
Opera pode ser atualizada sem reinstalar shell profiles ou configuração. O
gerenciamento nativo de dependências cobre Go modules; outros ecossistemas
continuam disponíveis pelas ferramentas do workspace e pelas instruções do
agente, sem um resolver genérico mantido pelo Opera.
