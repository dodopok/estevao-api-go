# Mapa de equivalência Rails → Go

O Rails (`estevao-api`) é a fonte da verdade. "Equivalente" aqui significa
**verificado contra o Rails em execução** pelo teste diferencial
([TESTING.md](TESTING.md)): mesma resposta (status, cabeçalhos, corpo byte a
byte) e mesmos efeitos persistidos (linhas do banco, jobs enfileirados,
objetos no armazenamento, chamadas a serviços externos). Nome ou estrutura
parecidos nunca bastaram para declarar equivalência.

## 1. HTTP

| Superfície | Rails | Go | Verificação |
|---|---|---|---|
| Rotas | `config/routes.rb`: 184 rotas, 176 endpoints | `internal/web/routes_table.go` (gerado de `rails routes` por `tools/gen/routes.rb`), mesma ordem de reconhecimento | 176/176 endpoints comparados — [ROUTE_COVERAGE.md](ROUTE_COVERAGE.md) |
| Engines montadas | Rswag UI + API em `/api-docs` | `internal/apidocs` (assets gerados por `tools/gen/api_docs.rb`, bytes idênticos) | suíte `basics` |
| Health | `/up`, `/ready` | `internal/app/basic.go` | suíte `basics` |
| Arquivos estáticos | `public/` via ActionDispatch::Static | `app.Static` | suíte `active_storage`/`basics` |
| Parâmetros | query/form/JSON, `wrap_parameters`, strong params | `internal/web/params*.go`, `permit.go` (tabela de wrap gerada) | todas as suítes |
| Erros | `rescue_from`, `ActionDispatch::PublicExceptions`, formatos por extensão/Accept | `internal/web/errors.go`, `exceptions.go` | todas as suítes (inclui casos 4xx/5xx) |
| Cache HTTP | `Rack::ETag`, `Rack::ConditionalGet`, `stale?`/`fresh_when` | `web.Server.conditional`, `conditional_get.go` | todas as suítes; ETag comparado byte a byte |
| CORS, HSTS, X-Request-Id, X-Runtime | rack-cors, `config.force_ssl`/HSTS, middlewares padrão | `internal/web/cors.go`, `server.go` | todas as suítes |
| Rate limit | `Rack::Attack` (IP, API key diário/minuto, servidor confiável) | `internal/ratelimit` — mesmas chaves no mesmo Redis em produção, `MemoryStore` fora de produção ou com `RACK_ATTACK_CACHE=memory` | suítes `developers`, `api_v2`, `admin_api_keys` |
| Autenticação | Firebase ID token (certificados do Google), App Check / `X-App-Internal-Id`, `X-API-Key`, admin por e-mail, `X-Trusted-Server-Key` | `internal/auth` | suítes `users*`, `api_v2`, `developers`, `dashboard` |

## 2. Domínio

| Área | Rails | Go |
|---|---|---|
| Calendário litúrgico (Páscoa, estações, cores, precedência, transferências, jejuns) | `app/services/liturgical*`, `Liturgical::PrayerBookRules` | `internal/liturgical` (regras por livro em `rules*.go`) |
| Lecionário, coletas, textos bíblicos | `app/services/reading`, `CollectService`, `Bible::*` | `internal/reading`, `internal/collects`, `internal/bible` |
| Ofício Diário (24 livros, todos os ofícios, rito familiar) | `DailyOfficeService`, `DailyOffice::Builders::*` | `internal/dailyoffice` (um arquivo por livro) |
| Preferências por livro | `Preferences::*`, `PreferenceDefinition` | `internal/prefs` |
| Fuso horário do usuário, "hoje" | `Time.zone`, `ActiveSupport::TimeZone` | `internal/rb` (port de `Date._parse`, `TimeZone#parse`), `internal/civil` |
| Áudio (trilha, catálogo, geração, admin) | `Audio::*` | `internal/audio`, `internal/audioadmin`, `internal/audiogen` |
| Usuários, conclusões, diário, favoritos, ofícios compartilhados, pedidos de oração, regras de vida, exames, rosário, música de fundo, assinaturas, notificações, desenvolvedores, cobrança Stripe, dashboard | controllers e serviços correspondentes | `internal/users`, `internal/api/v1/*`, `internal/liferules`, `internal/exams`, `internal/rosary`, `internal/bgmusic`, `internal/subscriptions`, `internal/notify`, `internal/developers`, `internal/billing`, `internal/dashboard` |
| API v2 | `app/controllers/api/v2`, `app/services/api/v2` | `internal/api/v2` |

## 3. Persistência

O Go usa **o mesmo banco e o mesmo schema** do Rails (ver
[DATABASE.md](DATABASE.md)): as mesmas tabelas, colunas, índices e
sequências; nenhuma migração nova. As escritas reproduzem o que o Active Record
grava (colunas, `updated_at` só quando algo muda, callbacks, `dependent:`,
contadores, `touch`), e as leituras reproduzem a ordem das consultas do Rails —
inclusive onde o resultado depende do plano do PostgreSQL (empates em `ORDER
BY`, `SELECT DISTINCT`).

## 4. Jobs (Solid Queue)

O Go lê e escreve as tabelas `solid_queue_*` exatamente como o Solid Queue 1.3
com a serialização do ActiveJob 8.1, então um job enfileirado por um lado é
executado pelo outro (verificado com `DIFF_CROSS_JOBS=1`). `retry_on`,
`discard_on`, execuções falhas, registro de processos, heartbeat e a agenda
recorrente seguem o Solid Queue.

| Classe | Go | Verificação |
|---|---|---|
| `BroadcastNotificationJob` | `internal/notify/job.go` | `notifications` |
| `CacheWarmerJob`, `CalendarWarmerJob` | `internal/workers/warmers.go` | `maintenance` |
| `DatabaseCleanupJob`, `CleanupExpiredSharedOfficesJob`, `FlushApiKeyUsageJob` | `internal/workers/maintenance.go` | `maintenance`, `developers` |
| `CustomRosaryPrayers::PublishJob`, `UnpublishJob`, `ReconcilePublicationJobsJob` | `internal/rosary/publication.go` | `custom_rosary` |
| `Audio::RecordUserUsageJob` | `internal/audio/usage.go` | `users`, `audio` |
| `ActiveStorage::AnalyzeJob`, `ActiveStorage::PurgeJob` | `internal/activestorage/attach.go` | `active_storage_writes`, `users_account` |
| `PrewarmOfficeAudioJob`, `GenerateBookAudioJob`, `RegenerateAudioClipJob`, `CleanupAudioClipsJob`, `IndexAudioCatalogJob` | `internal/audiogen/jobs.go` | `audio_jobs` (também cruzado) |
| `SolidQueue::RecurringJob` (comando) | `internal/solidqueue/recurring.go` | `maintenance` |
| `GenerateLiturgicalAudioJob` | **não portado** — só é executado *inline* pela rake `audio:generate` (fluxo legado do ElevenLabs), nunca pela fila | — |

Agenda (`config/recurring.yml`, produção): `clear_solid_queue_finished_jobs`,
`cache_warmer`, `calendar_warmer`, `database_cleanup`,
`reconcile_custom_rosary_publications`, `flush_api_key_usage` — todas
carregadas pelo `estevao-worker` (`internal/workers/recurring.go`), com o mesmo
formato de agenda (Fugit) e as mesmas linhas em `solid_queue_recurring_tasks`.

## 5. Tarefas rake

Nenhuma rake roda por agenda (`docs/deployment/scheduled_jobs.md` do Rails);
todas são operação manual. Situação:

| Tarefa | Situação |
|---|---|
| `db:seed` | portada: `estevao-seed load` + `seeds/` ([SEEDS.md](SEEDS.md)) |
| `cache:warm_calendar` (deploy) | portada: `estevao-worker -warm-calendar` |
| `cache:warm` | equivalente: enfileirar `CacheWarmerJob` (o worker Go executa) |
| `db:prepare` / migrações | **continuam no Rails** — o schema é dono das migrações ([DATABASE.md](DATABASE.md)) |
| `bible:*`, `psalters:seed`, `prayer_books:*`, `liturgical_texts:sync_catalog`, `import:collects` | continuam no Rails: importações e manutenção de dados de referência, rodadas contra o mesmo banco |
| `audio:*`, `office_audio:*`, `background_music:*`, `storage:avatars:*`, `feature_flags:*`, `notifications:*`, `life_rules:translate` | continuam no Rails: operação editorial/manual; a geração de áudio pelo admin já roda no Go |
| `benchmark:*`, `cache:stats/health/...`, `db:integrity:*`, `performance:analyze`, `redis:diagnostics`, `preferences:report`, `db:verify` | diagnósticos; substituídos por `cmd/bench`, `PPROF_ADDR` e consultas diretas |

O código Rails permanece no seu repositório e continua executável contra o
banco compartilhado, então nenhuma dessas tarefas se perde no corte.

## 6. Integrações

| Serviço | Uso | Go | Verificação |
|---|---|---|---|
| Firebase Auth (certificados, token de admin, exclusão de conta) | autenticação, exclusão de usuário | `internal/auth` | suítes com fakes (`fake_google.py`) |
| Firebase Cloud Messaging | notificações | `internal/notify/fcm.go` | `notifications` |
| RevenueCat | premium | `internal/subscriptions` | `subscriptions` |
| Stripe (Checkout, Billing Portal, webhooks assinados) | cobrança do portal | `internal/billing` | `billing`, `developers` |
| Perplexity | orações semanais, traduções | `internal/perplexity` | `maintenance`, `developers` |
| Strapi | publicação do rosário | `internal/rosary` | `custom_rosary` |
| OpenAI TTS, Google Cloud TTS (Gemini), ElevenLabs | geração de áudio | `internal/audio/synthesize.go` | `audio_jobs` (os três provedores) |
| S3 (Active Storage `railway_avatars`) | avatares, áudio, música | `internal/s3`, `internal/activestorage` | fake S3 em todas as suítes com arquivos |
| New Relic (APM, erros, métricas custom) | observabilidade | **não portado** — o Go registra JSON em stdout; o gancho `web.Server.ReportError` existe para ligar um agente | — |

## 7. Caches

Caches do Rails que não mudam o resultado de uma resposta (só o custo de
calculá-la) não precisam existir no Go para haver equivalência. Os que mudam o
que o cliente vê foram portados com a mesma chave e TTL (payloads da API v2,
catálogo de música, dashboard, resumo de áudio do admin, verificação de
assinatura). Os demais o Go calcula por requisição — ver desvio D6.

## 8. Desvios conhecidos

Todos foram avaliados como sem efeito para os clientes. Os que o teste
diferencial precisa tolerar estão codificados em `test/diff/diff.go`, com
referência a este documento.

| # | Desvio | Efeito | Por quê |
|---|---|---|---|
| D1 | Resposta 304 sem `Content-Length` (Rails manda `0`) e sem `Content-Type` quando uma engine Rack o manda em caixa mista | nenhum: 304 não tem corpo | o `net/http` remove esses cabeçalhos em 304 |
| D2 | Ordem de chaves de hashes montados de linhas lidas sem `ORDER BY` (`GROUP BY` do dashboard, `group_by` do mês do diário) | nenhum: a ordem também varia no próprio Rails entre execuções | o PostgreSQL não garante a ordem de agregação por hash; comparação feita como valor JSON |
| D3 | Backtrace de execuções falhas do Solid Queue | só diagnóstico | pilha Go em vez de Ruby; classe e mensagem iguais |
| D4 | Textos de erros de transporte (timeout, conexão recusada) nos logs e em `error_message` de operações | só diagnóstico | mensagens do Go em vez das do Ruby; classe de erro e código iguais |
| D5 | `Last-Modified` dos arquivos do Swagger UI | nenhum | é o mtime da instalação da gem; o Go usa o do momento da geração |
| D6 | Caches de desempenho do Rails (ofício base, dia do calendário, lecionário, preferências, feature flags por 1 min, dados do usuário) não existem no Go | depois de uma edição de dados, o Rails pode servir o valor antigo até o TTL; o Go responde já atualizado | cálculo por requisição; ver [PERFORMANCE.md](PERFORMANCE.md) para o custo |
| D7 | Os warmers (`CacheWarmerJob`, `CalendarWarmerJob`) executam as mesmas etapas e falham igual, mas aquecem só caches do processo Go | nenhum | consequência de D6 |
| D8 | O cache (5 min) dos multiplicadores de API key fica no Redis em chave própria do Go (o Rails guarda o seu em Marshal) | durante a convivência, uma mudança de plano feita por um lado só limpa o cache daquele lado; o outro converge em até 5 min | formatos de cache incompatíveis entre Ruby e Go |
| D9 | `DateZoneToDiff` (fusos por abreviação em `Date._parse`) cobre as abreviações de deslocamento fixo da `zonetab` | só entradas exóticas de data com nome de fuso | subconjunto usado pelos clientes |
| D10 | País inferido do fuso depende do `tzdata` do sistema | nenhum se a imagem tiver `tzdata` (o Dockerfile instala) | o Go lê `/usr/share/zoneinfo` |
| D11 | `User-Agent` das chamadas HTTP de saída | nenhum para os serviços usados | cliente HTTP diferente |
| D12 | Mensagens de falha de autorização do Google (`Signet::AuthorizationError`) | só diagnóstico | biblioteca diferente |
| D13 | `MOCK_PREMIUM` (usuário simulado só em `RAILS_ENV=development`) | nenhum em produção | conveniência de desenvolvimento não portada |
| D14 | New Relic ausente | perda de APM/alertas até ligar um agente | ver seção 6; risco listado no relatório final |
| D15 | Várias faixas de byte (`Range: bytes=0-1,5-9`) nos assets do Swagger UI são servidas inteiras | nenhum para navegadores | o Rack responderia `multipart/byteranges` |
