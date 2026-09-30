# estevao-api-go

Reimplementação em Go da API Rails [`estevao-api`](../estevao-api) (o backend do
app Ordo). O objetivo é substituir o Rails **sem mudança nos clientes**: mesmas
rotas, contratos JSON, códigos de erro, autenticação, validações, persistência
(o mesmo PostgreSQL e as mesmas tabelas) e regras litúrgicas. O Rails continua
sendo a fonte da verdade: cada comportamento aqui foi comparado com ele, rodando
lado a lado, pelo teste diferencial em `test/diff`.

| Documento | Conteúdo |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | A arquitetura de produção só com Go: processos, schema, conteúdo, jobs, observabilidade, testes sem o Rails e o checklist para apagá-lo |
| [docs/EQUIVALENCE.md](docs/EQUIVALENCE.md) | Mapa de equivalência (rotas, jobs, agenda, rake, integrações, caches) e desvios conhecidos |
| [docs/ROUTE_COVERAGE.md](docs/ROUTE_COVERAGE.md) | Cada rota do Rails, sua implementação em Go e quantas comparações a exercitam |
| [docs/TESTING.md](docs/TESTING.md) | O oráculo Rails, o teste diferencial e como rodá-lo |
| [docs/DATABASE.md](docs/DATABASE.md) | Estratégia de banco não destrutiva (schema, migrações, Solid Queue, caches) |
| [docs/SEEDS.md](docs/SEEDS.md) | O dataset canônico de seeds, o seeder em Go e a comparação com as fixtures |
| [docs/OPERATIONS.md](docs/OPERATIONS.md) | Desenvolvimento, verificação, deploy, corte de tráfego e rollback |
| [docs/PERFORMANCE.md](docs/PERFORMANCE.md) | Metodologia e resultados das medições Rails × Go |
| [docs/MIGRATION_REPORT.md](docs/MIGRATION_REPORT.md) | Relatório final: arquivos, comandos, cobertura, diferenças e riscos |

## Estrutura

```
cmd/
  estevao-api/      servidor HTTP (equivale ao Puma + Rails)
  estevao-worker/   worker de jobs sobre as tabelas do Solid Queue (equivale a bin/jobs)
  estevao/          operação: schema e migrações (db), conteúdo (seed), Bíblias, flags, caches
  difftest/         teste diferencial Rails × Go
  routecov/         relatório de cobertura por rota (docs/ROUTE_COVERAGE.md)
  bench/            medição de desempenho Rails × Go
internal/
  web/              pilha HTTP compatível com Rails (roteamento, params, erros, ETag, CORS)
  app/              montagem das rotas e endpoints
  api/v1, api/v2    controllers (finos) das duas versões da API
  liturgical/       calendário litúrgico: Páscoa, estações, cores, precedência, transferências
  reading/, collects/, bible/, dailyoffice/, prefs/, books/, explain/
                    lecionário, coletas, textos bíblicos, Ofício Diário, preferências por livro
  solidqueue/       fila compatível com Solid Queue 1.3 (enqueue, worker, dispatcher, agenda)
  workers/, audiogen/, notify/, rosary/, activestorage/ ...
                    jobs e serviços de domínio
  seed/             dataset canônico e o seeder
db/schema.sql       snapshot do schema do Rails (só para criar bancos novos)
seeds/              dataset canônico gerado a partir de `rails db:seed`
test/oracle/        oráculo Rails local, fakes de serviços externos
test/diff/          suítes do teste diferencial
tools/              geradores e scripts de verificação
```

Regras específicas de cada Livro de Oração ficam isoladas como no Rails: os
builders do Ofício Diário por livro (`internal/dailyoffice`), o `RuleSet` de
calendário por livro (`internal/liturgical/rules*.go`) e as capacidades
declaradas no próprio registro do livro (`internal/books`). Código compartilhado
não testa código de livro.

## Rodando

Pré-requisitos: Go 1.24, PostgreSQL 16, Redis e o pacote `tzdata` do sistema.

```bash
# banco novo (local): cria o banco, carrega db/schema.sql e seeds/
DATABASE_URL=postgres://localhost/estevao_dev go run ./cmd/estevao db prepare

# servidor e worker (mesmas variáveis de ambiente do Rails)
export DATABASE_URL=postgres://localhost/estevao_dev REDIS_URL=redis://localhost:6379/0 RAILS_ENV=development
go run ./cmd/estevao-api          # PORT (padrão 3000)
go run ./cmd/estevao-worker       # agenda de config/recurring.yml incluída
```

`estevao-worker -warm-calendar` equivale a `rails cache:warm_calendar` (o
`preDeployCommand` do Railway). `SOLID_QUEUE_IN_PUMA` roda os jobs dentro do
processo web, como o plugin do Puma.

## Configuração

As variáveis de ambiente são as do Rails (`.env.example` do repositório Rails) e
têm o mesmo significado; `RAILS_ENV` continua escolhendo o comportamento de
produção (armazenamento S3, rate limit em Redis, mensagens de erro). Variáveis
só do Go:

| Variável | Uso |
|---|---|
| `DB_MAX_CONNS` | tamanho do pool do PostgreSQL (padrão 20 no servidor, 10 no worker) |
| `PPROF_ADDR` | expõe o profiler do Go (ex.: `127.0.0.1:6060`); ausente, nada é servido |
| `PUBLIC_DIR` | diretório `public/` servido como arquivos estáticos |
| `*_API_URL` (`OPENAI_API_URL`, `GOOGLE_TTS_API_URL`, `ELEVENLABS_API_URL`, `REVENUECAT_API_URL`, `PERPLEXITY_API_URL`, `STRAPI_API_URL`, `STRIPE_API_URL`, `FCM_API_URL`, `FIREBASE_OAUTH_TOKEN_URL`, `FIREBASE_IDENTITY_TOOLKIT_URL`, `FIREBASE_CERTS_URL`) | endereço base de cada integração; em produção ficam ausentes (usam os endereços reais) e nos testes apontam para os fakes |

## Testes

```bash
go vet ./... && go test ./...                 # unidade e golden tests
tools/seed-verify.sh                          # seeder reconstrói seeds/ exatamente
test/oracle/restart.sh                        # sobe Rails (oráculo) + Go + fakes
go run ./cmd/difftest -suite all              # comparação Rails × Go (ver docs/TESTING.md)
```
