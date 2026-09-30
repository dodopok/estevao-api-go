# Arquitetura de produção (só Go)

Como a aplicação funciona em produção quando o Rails deixa de existir, o que
cada peça do Rails virou, e o que ainda depende dele até o dia em que for
apagado (seção 9).

## 1. Processos

Uma imagem (`Dockerfile`) com três binários:

| Binário | Papel | Equivalente Rails |
|---|---|---|
| `estevao-api` | servidor HTTP | Puma + Rails |
| `estevao-worker` | fila, dispatcher e agenda recorrente sobre as tabelas do Solid Queue; `-warm-calendar` | `bin/jobs`, `rails cache:warm_calendar` |
| `estevao` | ferramenta de operação: schema, conteúdo, Bíblias, flags, caches | `rails db:*`, as tarefas rake |

Em Railway, a imagem roda em dois serviços:
* **web:** `deploy/railway.toml`. Healthcheck em `/up`. O `preDeployCommand`
  roda `estevao db prepare && estevao-worker -warm-calendar`, antes de a versão
  receber tráfego.
* **worker:** `deploy/railway.worker.toml`.

Os dois leem as mesmas variáveis (seção 6).

Na partida, cada processo faz três coisas:
* ajusta `GOMAXPROCS` à cota de CPU do container e o limite de memória do
  coletor a 90% da memória do container (`internal/runtimecfg`). O Go 1.24 não
  lê nenhum dos dois do cgroup; `GOMAXPROCS`/`GOMEMLIMIT` explícitos prevalecem;
* recusa subir em produção sem `SECRET_KEY_BASE`, como o Rails;
* acusa em log um `TRUSTED_SERVER_KEY` igual ao `APP_INTERNAL_IDENTIFIER`.

O servidor tem `ReadHeaderTimeout` 10 s, `ReadTimeout` 60 s e `IdleTimeout`
75 s (maior que o do proxy de borda). O desligamento é gracioso: 30 s para as
requisições em curso, e o worker termina o job que está executando.

## 2. Dados

| Onde | O quê | Dono |
|---|---|---|
| PostgreSQL (um banco) | domínio, usuários, conteúdo litúrgico, Bíblias, fila (`solid_queue_*`), Active Storage | `estevao db` (schema), `estevao seed` (conteúdo), `estevao bible` (Bíblias) |
| Redis | caches (`estevao_api_v8:go/*`), rate limit (Rack::Attack), contadores de uso de API key | a aplicação |
| Bucket S3 (`railway_avatars`) | avatares, áudio, música de fundo | a aplicação, no formato do Active Storage |

O formato das linhas é o do Active Record:
* timestamps em UTC com microssegundos;
* enums como inteiros;
* `jsonb` com o mesmo shape;
* `updated_at` alterado só quando algo muda;
* `touch` e `dependent:` feitos explicitamente pelo código.

Os caches versionados pelo `updated_at` do livro se invalidam sozinhos
([EQUIVALENCE.md](EQUIVALENCE.md#7-caches)).

## 3. Schema: `estevao db`

As migrações são arquivos SQL em `db/migrations`, com seções up/down no formato
do dbmate. Elas são registradas **na mesma `schema_migrations` do Rails**, depois
da última migração Rails (a baseline). O banco de produção mantém um histórico
só, sem recomeçar.

O migrador usa o mesmo advisory lock do `ActiveRecord::Migrator`. Cada migração
roda numa transação com `lock_timeout` de 5 s. `db/schema.sql` vai embutido no
binário. `estevao db prepare` é o `rails db:prepare` e roda a cada deploy.

O CI verifica que `schema.sql` é exatamente o que as migrações produzem. Regras
de migração sem parar o serviço e o passo a passo:
[DATABASE.md](DATABASE.md#1-schema-e-migrações).

## 4. Conteúdo litúrgico: `seeds/` e `estevao seed`

`seeds/` é a fonte do conteúdo: livros, celebrações, textos, coletas, leituras,
salmos, ciclos, preferências, saltérios e o catálogo de música. Uma mudança de
conteúdo é uma edição em JSON, revisada em PR:

* **banco novo:** `estevao db prepare` carrega tudo, com os mesmos ids de um
  banco semeado pelo Rails;
* **banco existente:** `estevao seed sync [-book X] [-apply]` casa as linhas pela
  identidade natural, atualiza no lugar (mantendo o id), insere as novas,
  substitui grupos de coletas alterados e **nunca apaga**. Colunas de processo
  (áudio, publicação) não são sobrescritas. No fim, cada livro alterado é
  tocado, e os caches dele se renovam em todas as instâncias.

Detalhes e o procedimento do primeiro uso em produção: [SEEDS.md](SEEDS.md).

**Bíblias** (32 traduções, cerca de 1 milhão de versículos) ficam fora do
`seeds/`: várias têm direitos autorais e não entram no git. Elas se movem num
formato único, `estevao bible export` / `estevao bible import`. A importação
valida e substitui a tradução numa transação. Uma fonte nova só precisa ser
convertida para esse formato. O Rails tinha cinco importadores, um por fonte.

## 5. Jobs

A fila é compatível com o Solid Queue 1.3 e usa as mesmas tabelas, o mesmo
payload do ActiveJob e a agenda de `config/recurring.yml` (`internal/workers`).
* Os 17 jobs rodam no worker Go.
* `retry_on`/`discard_on` e `rescue_from` se comportam como no Rails.
* Os jobs são reivindicados com `FOR UPDATE SKIP LOCKED`; dois workers juntos
  não duplicam execuções.
* Jobs que falham ficam em `solid_queue_failed_executions`.

Manter o formato do Solid Queue permite rollback para o Rails, e também permite
trocar de fila no futuro sem pressa.

## 6. Configuração

As variáveis de ambiente são as do Rails (`DATABASE_URL`, `REDIS_URL`,
`SECRET_KEY_BASE`, Firebase, Stripe, RevenueCat, Strapi, S3, provedores de voz,
`TRUSTED_SERVER_KEY`, `APP_INTERNAL_IDENTIFIER`, …), com `RAILS_ENV=production`.

Específicas do Go:

| Variável | Uso |
|---|---|
| `DB_MAX_CONNS` | conexões por processo (padrão 20 no web, 10 no worker). Some as réplicas e fique abaixo do `max_connections` do PostgreSQL |
| `MIGRATION_LOCK_TIMEOUT` | espera máxima por lock de uma migração ou do `seed sync` (padrão `5s`) |
| `GOMAXPROCS`, `GOMEMLIMIT` | só para sobrepor o ajuste automático |
| `PPROF_ADDR` | expõe o profiler do Go num endereço interno |
| `ESTEVAO_TEST_CLOCK` | relógio de teste (seção 8). **Nunca em produção** |

## 7. Observabilidade

`internal/observe` envia ao New Relic o que o Rails enviava. Fica ligado quando
`NEW_RELIC_LICENSE_KEY` está definida; o resto da configuração vem das
variáveis `NEW_RELIC_*`, como no Rails.

* **Uma transação por requisição**, com o nome da rota (`api/v1/calendar/day`)
  e os atributos `controller`, `action`, `status`, `office_type` e
  `prayer_book`.
* **Uma transação por job.**
* **Consultas ao PostgreSQL** como segmentos (`nrpgx5`).
* **Erros** com os mesmos parâmetros do Rails: `controller`, `action`,
  `user_id`, `request_id`; nos jobs, `job_class`, `job_id` e `arguments`.
* **As métricas custom dos painéis:**
  * `Custom/API/<controller>#<action>/Duration` e `Custom/API/Status/<status>`;
  * `Custom/Jobs/<classe>/Duration|Success|Failure`;
  * `Custom/DailyOffice/Service/*/Duration`;
  * `Custom/CacheWarmer/*`.

O que muda em relação ao Rails:
* **Prefixo das transações:** o agente Go usa `WebTransaction/Go/…`, no lugar
  de `WebTransaction/Controller/…`. Consultas NRQL por `name` precisam do
  prefixo novo; por `request.uri` ou pelos atributos, não.
* **Métricas de cache do Rails** (`Custom/Cache/*`, `Custom/LeanCache/*`) não
  existem: descreviam o cache Ruby.
* **Logs:** JSON em stdout, lidos pela Railway. O encaminhamento de logs ao New
  Relic, que o Rails fazia, não foi ligado.

Durante a convivência, dê ao Go outro `NEW_RELIC_APP_NAME`, para não misturar
os dados dos dois lados. Depois do corte, voltar ao nome original mantém o
histórico dos painéis.

## 8. Testes sem o Rails

| Camada | O quê | Onde roda |
|---|---|---|
| Unitários e de pacote | `go test ./...` | CI, sempre |
| Banco | migrador, `seed sync`, round trip do seed, `schema.sql` × migrações | CI, job `database` |
| Corpus gravado | as respostas do Rails a 14 mil requisições e 58 cenários, gravadas uma vez, com o banco em que rodaram | `test/corpus/replay.sh` |
| Diferencial ao vivo | Rails × Go lado a lado (`cmd/difftest`) | enquanto o Rails existir |

O corpus substitui o oráculo quando o Rails for apagado.
* `test/corpus/record.sh` grava, onde o Rails roda: o dump do banco, a resposta
  normalizada do Rails a cada requisição e cenário, e o instante da gravação.
* `test/corpus/replay.sh` restaura o dump, sobe o servidor Go com o relógio
  (`internal/clock`) no instante gravado, e compara. O relógio vale para as
  datas, os tokens, o SQL das fixtures e os processos filhos.
* O corpus contém as Bíblias; por isso fica fora do git, num armazenamento
  privado (decisão pendente, seção 10).

Detalhes: [TESTING.md](TESTING.md).

## 9. O que ainda depende do Rails, e como apagá-lo

Hoje o Rails é usado só como **oráculo** e **fonte de geração**; nada em
produção depende dele. O que o usa:

| Peça | Para quê | Depois de apagar o Rails |
|---|---|---|
| `test/oracle/` (restart, run-oracle, `patches.rb`, `oracle.ru`) | o Rails lado a lado no teste diferencial | remover. Ficam os fakes (`fake_google.py`, `fake_s3.py`, `certs-www`), que o replay usa |
| `tools/gen/*.rb`, `tools/gen/run.sh` | extrair do Rails as tabelas geradas: rotas, livros da Bíblia, tabelas de leitura, prompts de áudio, params wrapper, erros do PostgreSQL | remover. Os 9 arquivos `*_gen.go`/`*_table.go` passam a ser código-fonte comum: tirar o cabeçalho `DO NOT EDIT` |
| `tools/golden/*.rb`, `test/golden` | fixtures geradas a partir do Rails | idem: as fixtures ficam como estão |
| `tools/seed-export.sh` | regenerar `seeds/` a partir dos seeds Ruby | remover: `seeds/` já é a fonte |
| `cmd/difftest` modo ao vivo | Rails × Go | fica o modo `-corpus` |

**Checklist para apagar o Rails**, depois do corte descrito em
[OPERATIONS.md](OPERATIONS.md):

1. Gravar o corpus final com o Rails de produção congelado
   (`test/corpus/record.sh`) e guardá-lo no armazenamento escolhido. Confirmar
   `test/corpus/replay.sh` = 0 diferenças.
2. Rodar `estevao seed sync` (sem `-apply`) contra produção e resolver as
   divergências (seção "Primeiro uso em produção" de [SEEDS.md](SEEDS.md)).
3. Exportar as Bíblias de produção (`estevao bible export`) para o mesmo
   armazenamento privado: é o backup de onde uma tradução é reimportada.
4. Ligar o job de replay no CI, com o corpus baixado de um segredo.
5. Remover as peças da tabela acima e as referências nos documentos.
6. A partir daí, qualquer mudança de comportamento que mude uma resposta
   gravada exige gravar de novo. Sem o Rails, a gravação passa a ser feita a
   partir do Go: `difftest` com o Go nos dois lados. Uma mudança **intencional**
   é aprovada revendo a diferença, como numa mudança de contrato.

## 10. Decisões que ficam com vocês

| Decisão | Recomendação |
|---|---|
| Onde guardar o corpus gravado e o backup das Bíblias (dados com direitos autorais; ~60 MB por gravação) | artefato privado: *release asset* privado do repositório, ou bucket, com a URL num segredo do CI |
| `estevao seed sync -apply` no `preDeployCommand` | só depois do primeiro sync manual em produção e de algumas publicações revisadas à mão |
| Quem é dono das migrações enquanto o Rails existir | o Rails, até o corte; depois, só o Go. Os dois usam o mesmo lock, e uma migração nunca roda em paralelo, mas duas equipes migrando o mesmo banco é o que se quer evitar |
| `NEW_RELIC_APP_NAME` do Go | nome próprio na convivência; o nome original depois do corte |
| Encaminhar logs ao New Relic | só se os alertas atuais dependem de logs; senão, bastam os logs da Railway |

## 11. O que foi deixado de fora de propósito

* **Tipagem do domínio.** O código espelha o Ruby: mapas ordenados (`rb.Map`),
  panics no lugar de exceções, semântica Ruby em `internal/rb`. É isso que
  garante respostas idênticas byte a byte. Trocar por structs e erros como
  valores vale a pena, mas pacote a pacote, com o corpus como rede. Não é
  pré-requisito para operar só com Go.
* **ORM ou gerador de SQL.** As cerca de 350 consultas escritas à mão continuam.
  `sqlc` é o caminho recomendado quando forem reescritas, junto com a tipagem.
* **Trocar a fila.** O formato do Solid Queue é o que torna o rollback trivial;
  trocar só depois que a janela de rollback fechar.
