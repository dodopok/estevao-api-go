# Relatório final da migração Rails → Go

Estado em 2026-09-29, branch `claude/loving-brown-bn3em2` do repositório
`estevao-api-go`. O repositório Rails não foi alterado (só artefatos ignorados
pelo git — logs e `tmp/` — foram gerados ao rodá-lo como oráculo).

## 1. Resultado

* **Todos os 176 endpoints** das 184 rotas do Rails estão implementados e
  **cada um é comparado** com o Rails em execução por pelo menos uma requisição
  ([ROUTE_COVERAGE.md](ROUTE_COVERAGE.md)); também `/api-docs` (Rswag),
  `/up`, `/ready` e arquivos estáticos.
* **Regressão completa: 0 diferenças** em 14.089 requisições e 58 cenários de
  escrita/jobs (28 suítes), com cache frio e quente — ver seção 3.
* **Jobs:** todas as 17 classes enfileiráveis rodam no worker Go sobre as
  tabelas do Solid Queue, e cada lado executa jobs enfileirados pelo outro.
  Agenda recorrente de produção completa.
* **Seeds:** dataset canônico único (`seeds/`) e seeder Go que reproduz o
  banco semeado pelo Rails **linha a linha, com os mesmos ids**, em ~4 s (o
  `db:seed` do Rails leva ~12 min).
* **Banco:** nenhuma migração, nenhum DDL, nenhum dado convertido; Rails e Go
  podem rodar juntos contra o mesmo banco ([DATABASE.md](DATABASE.md)).
* **Desempenho:** o Go é mais rápido em todos os 11 endpoints medidos — 1,7× a
  6,7× a vazão do Rails com cache, 6× a 23× menos latência sem cache — usando
  72 MB de memória contra 707 MB ([PERFORMANCE.md](PERFORMANCE.md)).

A migração **não** está marcada como concluída para produção: o corte depende
das decisões e verificações da seção 6 e do procedimento de
[OPERATIONS.md](OPERATIONS.md).

## 2. Arquivos criados

~290 arquivos Go (~73 mil linhas), organizados em:

| Onde | O quê |
|---|---|
| `cmd/estevao-api`, `cmd/estevao-worker`, `cmd/estevao` | servidor HTTP, worker de jobs (+ `-warm-calendar`), ferramenta de operação (schema, seeds, Bíblias, flags, caches) |
| `cmd/difftest`, `cmd/routecov`, `cmd/bench` | teste diferencial, relatório de cobertura por rota, benchmark |
| `internal/web`, `internal/app`, `internal/rb`, `internal/rx`, `internal/ar` | pilha HTTP compatível com Rails/Rack e ports de semântica Ruby/ActiveSupport/Active Record usados pelos contratos (formatação de JSON, datas, `Date._parse`, regex Onigmo, casts) |
| `internal/api/v1`, `internal/api/v2` | controllers |
| `internal/liturgical`, `calgrid`, `reading`, `collects`, `bible`, `dailyoffice` (um arquivo por livro), `prefs`, `books`, `explain` | domínio litúrgico |
| `internal/users`, `auth`, `ratelimit`, `developers`, `billing`, `subscriptions`, `notify`, `rosary`, `bgmusic`, `liferules`, `exams`, `prayerrequests`, `dashboard`, `wrapped`, `activestorage`, `s3`, `audio`, `audioadmin`, `audiogen`, `perplexity`, `integrations` | demais áreas e integrações |
| `internal/solidqueue`, `internal/workers` | fila compatível com Solid Queue e os jobs |
| `internal/seed`, `seeds/`, `db/schema.sql` | seeds e schema de referência |
| `internal/apidocs` | `/api-docs` (assets gerados do Rails) |
| `test/oracle`, `test/effects`, `test/diff` (28 suítes), `test/golden` | oráculo Rails, fakes, suítes diferenciais, golden files |
| `tools/gen`, `tools/golden`, `tools/*.sh` | geradores a partir do Rails, scripts de schema/seed |
| `Dockerfile`, `deploy/*.toml`, `.github/workflows/ci.yml` | imagem, serviços Railway, CI |
| `README.md`, `docs/*.md` | documentação |

## 3. Comandos executados e resultados

| Comando | Resultado |
|---|---|
| `go vet ./...`, `gofmt -l` | limpo |
| `go test ./...` (com e sem banco) | todos os pacotes passam |
| `go run ./cmd/difftest -suite all` | 28 suítes, 14.089 requisições + 58 cenários, **0 diferenças** (rodado duas vezes na versão final: depois dos caches do dia/ofício e depois dos caches de grade/lecionário) |
| `difftest -suite calendar_grid,lectionary,catalog,api_v2` com o cache do Go vazio e de novo com ele cheio | 5.452 requisições em cada passada, 0 diferenças |
| `DIFF_CROSS_JOBS=1 ... -suite audio_jobs,admin_audio` (e as demais suítes de jobs ao longo do trabalho) | 0 diferenças |
| `go run ./cmd/routecov` | 176/176 endpoints implementados e comparados |
| `tools/seed-verify.sh` e comparação tabela a tabela com o banco semeado pelo Rails | idênticos (21 tabelas, ids e sequências incluídos) |
| `go run ./cmd/bench -c 8 -d 10s` | ver [PERFORMANCE.md](PERFORMANCE.md) |
| `CGO_ENABLED=0 go build` dos três binários | ok |
| `docker build` | **não executado**: sem daemon Docker neste ambiente; o CI tem um job para isso |

## 4. Cobertura por endpoint

[ROUTE_COVERAGE.md](ROUTE_COVERAGE.md) lista as 184 rotas com o número de
requisições comparadas e as suítes. As maiores: dia do calendário (3.648),
lecionário (3.008), Ofício Diário (2.441), explicações (2.232), API v2 (1.225).
Escritas, jobs e integrações são cobertos pelos 58 cenários (estado do banco,
jobs enfileirados, objetos no S3 e chamadas aos serviços externos comparados).

## 5. Diferenças não resolvidas

Todas documentadas em [EQUIVALENCE.md](EQUIVALENCE.md#8-desvios-conhecidos)
(D1–D15). Nenhuma altera o conteúdo de uma resposta a um cliente; as que
importam para operação:

* **D14 — New Relic não portado.** O Go registra JSON em stdout; APM, erros e
  métricas custom (`Custom/CacheWarmer/*` etc.) deixam de ser enviados.
* **D6 — caches só de desempenho** que o Go não tem: depois de uma edição de
  dados, o Go responde atualizado onde o Rails ainda serviria o valor antigo
  até o TTL.
* **D8 — cache de multiplicadores de API key** com chave própria: durante a
  convivência, uma mudança de plano feita por um lado leva até 5 min para valer
  no outro.
* Tarefas rake continuam no Rails (nenhuma é agendada); `GenerateLiturgicalAudioJob`
  (só usado inline pela rake legada `audio:generate`) não foi portado.

## 6. Riscos para produção e o que decidir antes do corte

| Risco | Mitigação |
|---|---|
| Perda de observabilidade (D14) | decidir entre alertas por logs e um agente APM no gancho `ReportError` **antes** do canário |
| O oráculo cobre o que as suítes exercitam; combinações de dados de produção não vistas podem divergir | fase sombra só de leitura contra produção e canário gradual ([OPERATIONS.md](OPERATIONS.md#4-corte-de-tráfego-gradual)) |
| Dados de produção diferentes dos seeds locais (edições feitas direto no banco) | o Go lê o banco de produção como está; a fase sombra compara com o Rails sobre esses dados |
| Imagem Docker não construída aqui | o job `docker` do CI; o `tzdata` é obrigatório (D10) |
| Dois workers ligados ao mesmo tempo | seguro (`SKIP LOCKED`, execuções recorrentes únicas), mas deixe só um depois da troca |
| Mudanças futuras no Rails antes do corte | regenerar tabelas (`tools/gen/run.sh`), schema e seeds e repetir o `difftest` completo |
| Custo de CPU nos endpoints sem cache (D6) | medido: o Go é mais rápido também sem cache ([PERFORMANCE.md](PERFORMANCE.md)) |

Nada foi implantado, nenhum tráfego foi desviado e nenhum dado de produção foi
lido ou alterado.
