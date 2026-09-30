# Testes: o Rails como oráculo, e depois dele

A equivalência é provada comparando as duas aplicações em execução, lado a
lado, contra o mesmo banco. O repositório Rails não é modificado: o oráculo o
carrega a partir do seu próprio caminho e só substitui, em memória, os destinos
das integrações externas pelos fakes locais.

## Peças

| Peça | Arquivo | Papel |
|---|---|---|
| Oráculo Rails | `test/oracle/run-oracle.sh`, `oracle.ru`, `patches.rb` | Rails 8.1 / Ruby 3.2.3, `RAILS_ENV=production`, Puma single-mode na porta 3000, Redis db 1; `LD_PRELOAD=stable_qsort.so` reproduz o `qsort` estável da glibc de produção |
| Servidor Go | `test/oracle/run-go.sh` | porta 3001, Redis db 2, mesmas variáveis (`oracle.env`) |
| Worker Go | `test/oracle/run-go-worker.sh` | executa jobs (`-drain`) com o ambiente do Go |
| Jobs no Rails | `test/effects/perform_jobs.rb`, `enqueue_jobs.rb` | executam os jobs que o oráculo enfileirou (via `rails runner`) |
| Fakes | `fake_google.py` (Google token, FCM, Firebase Admin, RevenueCat, Perplexity, Strapi, Stripe, OpenAI/Google/ElevenLabs TTS), `fake_s3.py`, `certs-www/` (certificados Firebase) | respostas determinísticas e registro das chamadas recebidas |
| Chaves de teste | `gen-keys.sh`, `test_firebase_*.pem` | par de chaves gerado localmente só para assinar tokens de teste; não é credencial de nenhum serviço |
| Fixtures de cenário | `test/diff/fixtures/*.sql` | dados de teste com ids fixos, aplicados antes de subir |

`test/oracle/bootstrap-db.sh` cria o banco local do oráculo do zero (schema e
`db:seed` do Rails, depois as importações de Bíblia). Recusa qualquer
`DATABASE_URL` que não seja local.

## Rodando

```bash
test/oracle/restart.sh                               # sobe fakes, oráculo e Go; aplica fixtures; ANALYZE
set -a; source test/oracle/oracle.env; set +a
RAILS_RUNNER=/caminho/rails-runner.sh go run ./cmd/difftest -suite all
go run ./cmd/difftest -suite daily_office,api_v2 -show 5
DIFF_CROSS_JOBS=1 RAILS_RUNNER=... go run ./cmd/difftest -suite audio_jobs   # Go executa os jobs do Rails e vice-versa
go run ./cmd/difftest -suite audio_jobs -dump /tmp/dump                      # grava passos e snapshots de cada lado
```

`RAILS_RUNNER` é um script que roda `bin/rails runner "$@"` no repositório Rails
com o mesmo ambiente do oráculo (ver o exemplo em `run-oracle.sh`).

## Corpus gravado: testar o Go sem o Rails

O teste diferencial precisa do Rails rodando. O corpus guarda as respostas dele
para que o Go continue verificável depois que o Rails for apagado.

```bash
# onde o oráculo roda (grava e compara ao vivo ao mesmo tempo)
RAILS_RUNNER=/caminho/rails-runner.sh test/corpus/record.sh /algum/lugar/corpus
# em qualquer máquina com PostgreSQL 16, Redis e Python 3 (sem Ruby)
test/corpus/replay.sh /algum/lugar/corpus
test/corpus/replay.sh /algum/lugar/corpus -suite calendar_day,daily_office
```

O diretório do corpus tem três partes:
* **`database.dump`:** o banco no instante da gravação (`pg_dump -Fc`), com as
  Bíblias importadas e as fixtures;
* **`<suíte>.jsonl.gz`:** a resposta **do Rails** a cada requisição e cenário,
  já normalizada como a comparação ao vivo normaliza (os mesmos campos
  voláteis), com os snapshots de banco, S3 e jobs de cada cenário;
* **`meta.json`:** o instante da gravação e os commits do Go e do Rails.

O `record.sh` recusa gravar perto da virada do dia (23h–1h e 2h–4h UTC). O
replay é mais rápido que a gravação e o relógio dele fica um pouco atrás; uma
meia-noite nesse intervalo mudaria a data de algumas requisições.

### O relógio de teste

As respostas dependem de "hoje":
* `/calendar/today`;
* sequências de orações e anotações;
* expiração de tokens;
* validade de URLs assinadas.

O replay acontece dias ou meses depois da gravação. Por isso **toda leitura do
relógio passa por `internal/clock`**. Com `ESTEVAO_TEST_CLOCK=<instante RFC
3339>`, o processo acredita que o tempo começou naquele instante e continua
correndo dali.

O `replay.sh` liga isso no servidor Go e no `difftest`:
* o `difftest` assina os tokens com esse relógio;
* os processos que ele inicia (os workers dos cenários) herdam o instante atual
  (`clock.ChildEnv`);
* o SQL das fixtures troca `now()`, `CURRENT_DATE` e similares pelo instante do
  relógio (`diff.ClockSQL`), porque o PostgreSQL não tem como ser enganado;
* a verificação de JWT usa o mesmo relógio (`jwt.WithTimeFunc`).

O código de produção nunca chama `time.Now()` diretamente. Em produção a
variável não existe e o relógio é o real.

### Quando gravar de novo

* Uma suíte mudou (requisição nova, cenário novo). O replay acusa `STALE` e
  pede nova gravação.
* Uma mudança **intencional** de comportamento. Enquanto o Rails existir,
  grave a partir dele. Depois, grave a partir do Go revisado:
  `difftest -rails <go> -go <go> -record ...`; a revisão da diferença faz o
  papel do oráculo.

O corpus contém textos bíblicos com direitos autorais e não entra no git
(`.gitignore`). Onde guardá-lo:
[ARCHITECTURE.md](ARCHITECTURE.md#10-decisões-que-ficam-com-vocês).

## Como uma comparação funciona

**Suítes de requisições** (`register`): cada requisição vai aos dois servidores;
status, todos os cabeçalhos (exceto `Date`, `X-Request-Id`, `X-Runtime`,
`Last-Modified`) e o corpo são comparados byte a byte.

**Suítes de cenários** (`registerScenarios`): para cada lado, o cenário parte do
mesmo estado — aplica `Setup` (SQL), rebobina as sequências das `Tables` (os dois
lados criam os mesmos ids), limpa o Redis do lado, executa os passos, registra os
jobs enfileirados (classe, fila, payload do ActiveJob), executa-os (`Settle`) e
tira os `Snapshot`s (consultas SQL e respostas HTTP, como a lista de objetos do S3
falso ou as chamadas recebidas pelos fakes). Tudo isso é comparado.

**Normalização** — só valores voláteis por natureza:

| Normalizado | Motivo |
|---|---|
| `request_id`, `trace_id` e as chaves `Volatile` de cada requisição (ex.: `created_at` de um registro criado agora) | relógio/aleatoriedade |
| `X-Amz-Date` e `X-Amz-Signature` de URLs pré-assinadas | assinatura depende do segundo |
| timestamps no `DETAIL` de erros do PostgreSQL | relógio |
| ETag e `Content-Length` quando o corpo teve algo normalizado | derivam do corpo |
| `AnyKeyOrder` (dashboard) | ordem de `GROUP BY` não é estável nem no Rails (desvio D2) |
| `Scrub` (sufixo aleatório do arquivo de um candidato de áudio) | `SecureRandom.hex` |

Nada além disso é tolerado; os desvios aceitos estão em
[EQUIVALENCE.md](EQUIVALENCE.md#8-desvios-conhecidos).

## Suítes

| Suíte | Tipo | Tamanho | Cobre |
|---|---|---:|---|
| `calendar_day` | requisições | 3.648 | dia do calendário, todos os livros, anos e datas limite |
| `calendar_grid` | requisições | 780 | mês, ano, visão geral, estações, datas-chave |
| `lectionary` | requisições | 3.008 | leituras e ciclos por livro |
| `daily_office` | requisições | 2.441 | Ofício Diário: 24 livros, ofícios, rito familiar, preferências, trilha de áudio |
| `explanations` | requisições | 2.232 | explicações litúrgicas |
| `api_v2` | requisições | 1.225 | API v2 inteira (includes, fields, cursores, erros problem+json) |
| `catalog` | requisições | 439 | livros, preferências, versões bíblicas, celebrações |
| `active_storage` | requisições | 222 | blobs, representações, redirecionamentos, proxies |
| `audio`, `basics` | requisições | 45 / 49 | áudio público, health, `/api-docs`, 404 por formato |
| `users`, `users_account`, `user_content`, `life_rules`, `life_rule_exams`, `prayer_requests`, `notifications`, `subscriptions`, `background_music`, `custom_rosary`, `developers`, `billing`, `dashboard`, `admin_api_keys`, `admin_audio`, `audio_jobs`, `maintenance`, `active_storage_writes` | cenários | 58 cenários | escritas, jobs, integrações, admin |

Além disso, `go test ./...` roda testes de unidade e *golden tests* gerados do
Rails (`tools/golden/*.rb`): linhas do Ofício por livro e data, referências
bíblicas, coletas, normalização de texto para áudio e chaves de clipe.

## Relatórios

* `go run ./cmd/routecov > docs/ROUTE_COVERAGE.md` — cada rota do Rails, se o Go
  a implementa e quantas requisições das suítes a comparam.
* `tools/seed-verify.sh` — o seeder Go reconstrói `seeds/` exatamente.
* `go run ./cmd/bench` — ver [PERFORMANCE.md](PERFORMANCE.md).
