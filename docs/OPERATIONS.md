# Operação: desenvolvimento, verificação, deploy e rollback

> Nada neste repositório executa deploy, corte de tráfego ou migração de dados
> por conta própria. Os passos abaixo são para uma pessoa executar, na ordem, com
> os pontos de decisão marcados.

## 1. Desenvolvimento

```bash
export DATABASE_URL=postgres://localhost/estevao_dev REDIS_URL=redis://localhost:6379/0
go run ./cmd/estevao db prepare   # cria o banco, carrega schema e seeds (~5 s)
go run ./cmd/estevao-api          # :3000
go run ./cmd/estevao-worker       # fila e agenda
```

Mudança de schema: `estevao db new <nome>`, `estevao db migrate`, `estevao db
dump` ([DATABASE.md](DATABASE.md)). Mudança de conteúdo: editar `seeds/` e
`estevao seed sync` ([SEEDS.md](SEEDS.md)).

Antes de cada commit: `gofmt -l cmd internal test` vazio, `go vet ./...`,
`go test ./...`. Os testes de banco precisam de
`MIGRATE_TEST_DATABASE_URL=postgres://.../postgres`. Mudança que possa afetar
respostas: rode as suítes diferenciais afetadas, ao vivo ou contra o corpus
gravado ([TESTING.md](TESTING.md)).

## 2. Verificação antes de um release

1. `go vet ./... && go test ./...` e `tools/seed-verify.sh` (o CI já roda).
2. Oráculo atualizado: o `RAILS_ROOT` aponta para o commit do Rails que está em
   produção; `test/oracle/restart.sh`.
3. `go run ./cmd/difftest -suite all` — **0 diferenças** é critério de release.
   Em seguida `DIFF_CROSS_JOBS=1 ... -suite audio_jobs,users,custom_rosary,notifications,maintenance`
   (cada lado executa os jobs do outro).
4. `go run ./cmd/routecov` — nenhuma rota com `**no**` ou `**0**`.
5. `go run ./cmd/bench` — sem regressão frente à medição anterior
   ([PERFORMANCE.md](PERFORMANCE.md)).
6. Se o Rails mudou desde a última geração: `tools/gen/run.sh` (tabelas
   extraídas do Rails: rotas, prompts, regras), `tools/schema-dump.sh` e, se os
   seeds mudaram, `tools/seed-export.sh` — e revisar os diffs.

## 3. Deploy (serviços paralelos, sem corte)

A imagem (`Dockerfile`) contém `estevao-api`, `estevao-worker` e
`estevao`. Em Railway, dois serviços novos a partir deste repositório,
usando **as mesmas variáveis compartilhadas** do Rails (`DATABASE_URL`,
`REDIS_URL`, Firebase, Stripe, RevenueCat, Strapi, S3, TTS, `TRUSTED_SERVER_KEY`,
`APP_INTERNAL_IDENTIFIER`, etc.) e `RAILS_ENV=production`:

* web: `deploy/railway.toml` (healthcheck `/up`, `preDeployCommand` =
  `estevao db prepare && estevao-worker -warm-calendar`);
* worker: `deploy/railway.worker.toml` — **mantenha-o desligado** até a etapa 4.3.

O `estevao db prepare` do deploy aplica as migrações do Go. Enquanto
`db/migrations` estiver vazio, ele só confere que o banco tem a baseline Rails.
Se não tiver, o deploy falha em vez de subir contra um schema mais antigo.
Durante a convivência, o Rails e o Go usam o mesmo advisory lock de migração,
e uma migração nunca roda em paralelo à outra. Quem cria migrações nesse
período é uma decisão ([ARCHITECTURE.md](ARCHITECTURE.md#10-decisões-que-ficam-com-vocês)).

## 4. Corte de tráfego (gradual)

> **Ponto de decisão.** Cada etapa só avança com os critérios atendidos. Se
> algum falhar, siga o rollback (seção 5) da etapa em que está.

1. **Sombra (0% de tráfego).** O web Go no ar, recebendo só chamadas internas:
   repita o `difftest` apontando `-go` para o serviço Go e `-rails` para o Rails
   de produção **apenas com suítes de leitura** (`calendar_*`, `lectionary`,
   `daily_office`, `explanations`, `catalog`, `api_v2`, `basics`, `audio`). As
   suítes de cenário escrevem no banco e **não** devem rodar contra produção.
2. **Canário.** Direcione uma fração do tráfego do domínio para o serviço Go
   (proxy/balanceador na frente dos dois serviços). Compare taxa de 5xx,
   latência p95 e os logs `[ERROR]` dos dois lados por pelo menos um ciclo diário
   (os jobs agendados de madrugada precisam ter rodado no worker Rails sem
   interferência). Rails e Go servindo juntos é seguro: mesmo banco, mesmos
   contadores de rate limit, jobs intercambiáveis ([DATABASE.md](DATABASE.md)).
3. **Worker.** Ligue o worker Go e, depois de ver uma execução de cada tarefa
   recorrente nos logs, desligue o worker Rails. Os dois juntos por alguns minutos
   não duplicam execuções (`SKIP LOCKED`, execuções recorrentes únicas por
   `run_at`). Antes de desligar o Rails, confirme que não há
   `GenerateLiturgicalAudioJob` pendente.
4. **100%.** Todo o tráfego no Go. Mantenha os serviços Rails existindo (parados
   ou com 0 réplicas) durante a janela de rollback combinada.
5. **Observabilidade.** O Go envia transações, erros e as métricas custom ao New
   Relic quando `NEW_RELIC_LICENSE_KEY` está definida
   ([ARCHITECTURE.md](ARCHITECTURE.md#7-observabilidade)). Na convivência, dê
   ao serviço Go um `NEW_RELIC_APP_NAME` próprio. Revise os alertas que filtram
   por nome de transação: o prefixo passa a ser `WebTransaction/Go/`.

## 5. Rollback

O rollback é sempre **trocar processos**; nenhum dado precisa ser convertido,
porque os dois lados leem e escrevem os mesmos formatos.

| Situação | Ação |
|---|---|
| Web Go com problema (qualquer etapa) | devolver 100% do tráfego ao serviço Rails; o Go pode continuar no ar sem tráfego para investigação |
| Worker Go com problema | religar o worker Rails (`./bin/jobs`) e desligar o Go; jobs que o Go enfileirou são executados pelo Rails, e vice-versa; jobs que falharam ficam em `solid_queue_failed_executions` como no Solid Queue |
| Dados escritos pelo Go durante o canário | nada a fazer: são registros no formato do Active Record (verificado pelo `difftest`) |
| Caches | o Rails reconstrói os seus; o Go só apaga entradas do Rails que o próprio Rails apagaria |

Depois de desligar o Rails em definitivo, o repositório Rails continua sendo a
ferramenta das migrações e das rakes de manutenção listadas em
[EQUIVALENCE.md](EQUIVALENCE.md#5-tarefas-rake), até que sejam substituídas.

## 6. Tarefas de operação (as rake do Rails)

| Rails | Go |
|---|---|
| `rails db:prepare`, `db:migrate`, `db:rollback`, `db:migrate:status` | `estevao db prepare`, `migrate`, `rollback`, `status` |
| `rails g migration` + `db:schema:dump` | `estevao db new <nome>` + `estevao db dump` |
| `rails db:seed` | `estevao db prepare` (banco novo) ou `estevao seed load` |
| `prayer_books:seed[código]`, `prayer_books:seed_all`, `prayer_books:setup_dwdo`, `liturgical_texts:sync_catalog`, `import:collects`, `psalters:seed`, `background_music:seed`, `prayer_books:seed_psalm_translation_preferences` | editar `seeds/` e `estevao seed sync [-book código] -apply` |
| `bible:*` (download, import, setup, stats, clear, reimport) | `estevao bible export` / `import [-replace]` / `stats`; uma fonte nova é convertida para o formato do arquivo ([ARCHITECTURE.md](ARCHITECTURE.md#4-conteúdo-litúrgico-seeds-e-estevao-seed)) |
| `feature_flags:list/enable/disable/reset` | `estevao flags list`; `estevao flags enable`, `disable` ou `reset` `FEATURE [global \| user ALVO]` |
| `cache:clear_all`, `cache:clear`, `cache:clear_daily_office` | `estevao cache clear [-pattern daily_office/*]`; apaga só os caches e preserva rate limit e contadores de uso |
| `cache:warm`, `cache:warm_calendar` | `estevao cache warm` (enfileira `CacheWarmerJob`), `estevao-worker -warm-calendar` |
| `prayer_books:touch[códigos]` | `estevao books touch códigos...` |
| `notifications:test_notification[email]` | `estevao notifications test email` |
| `notifications:send_streak_reminders`, `cleanup_old_tokens` | jobs agendados no worker, como no Rails |
| `audio:*`, `office_audio:*` | a geração de áudio é operada pelos endpoints `api/v1/admin/audio/*` e pelos jobs do worker; as rakes de sincronização de arquivos locais (`audio:sync`, `upload_to_railway`) eram de uma instalação anterior ao bucket |
| `avatars:storage:*`, `background_music:import/withdraw/verify` | migrações pontuais já executadas; importar música nova continua exigindo transcodificação fora da aplicação |
| `benchmark:*`, `cache:stats/health/performance`, `db:integrity:*`, `db:verify`, `performance:analyze`, `redis:diagnostics`, `preferences:report` | diagnósticos: `cmd/bench`, `PPROF_ADDR`, New Relic e consultas SQL diretas |
| `life_rules:translate` | tradução pontual das regras embutidas; o resultado está em `seeds/` |
