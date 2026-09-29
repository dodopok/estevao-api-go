# Desempenho: Rails × Go

## Metodologia

`go run ./cmd/bench` (código em `cmd/bench/main.go`). Para ser justo:

* **Mesma máquina, mesmo banco, mesmo Redis.** Os dois servidores, o
  PostgreSQL 16, o Redis e o gerador de carga dividem a mesma máquina de 4 vCPUs
  e 16 GB; nenhum lado tem recurso exclusivo.
* **Rails configurado como em produção:** `config/puma.rb` sem alterações —
  `WEB_CONCURRENCY=2` workers × `RAILS_MAX_THREADS=5`, `preload_app!`,
  `RAILS_ENV=production`, Ruby 3.2.3 (sem YJIT, como a imagem de produção). O Go
  roda com os padrões (`GOMAXPROCS` = 4, pool de 20 conexões).
* **Mesmas respostas.** Antes de medir um endpoint, as duas respostas precisam
  ser `200` com o mesmo corpo (só `meta.generated_at` da v2 é ignorado); se
  diferirem, o endpoint não é comparado.
* **Aquecimento e alternância.** Cada endpoint recebe 20 requisições de
  aquecimento nos dois lados; depois é medido num lado e no outro, alternando
  quem vai primeiro a cada endpoint.
* **Carga:** 8 clientes concorrentes com keep-alive, 10 s por endpoint e por
  servidor; latência de cada requisição e vazão total. Erros e respostas não-200
  são contados à parte.
* **Frio:** com os dois caches Redis esvaziados, 20 datas distintas por endpoint,
  uma requisição por vez — o custo de calcular sem cache.
* Autenticação e rate limit iguais para os dois: `X-App-Internal-Id` +
  `X-Trusted-Server-Key` (v1) e `X-API-Key` (v2).

Limites: é uma máquina compartilhada, não o ambiente de produção (Railway),
e mede leitura; as escritas não entram (seu custo é dominado pelo banco, igual
para os dois).

## Resultados (2026-09-29, commit `b8793bd`)

`go run ./cmd/bench -c 8 -d 10s`, com o Rails de benchmark em `:3100` e o Go em
`:3001`. Todos os endpoints passaram na verificação de corpo idêntico; nenhum
erro nem resposta não-200.

### Com cache (vazão, 8 clientes)

| Endpoint | Rails req/s | Go req/s | Go ÷ Rails | Rails p95 ms | Go p95 ms |
|---|---:|---:|---:|---:|---:|
| dia do calendário | 293 | 1.395 | 4,8× | 40,9 | 10,3 |
| mês do calendário | 276 | 1.448 | 5,2× | 43,7 | 10,1 |
| ano do calendário | 216 | 789 | 3,7× | 54,1 | 18,4 |
| lecionário do dia | 325 | 1.589 | 4,9× | 39,2 | 9,1 |
| Ofício Diário `loc_2015` manhã | 252 | 715 | 2,8× | 48,4 | 19,6 |
| Ofício Diário `loc_2019_en` tarde | 264 | 619 | 2,3× | 47,1 | 23,0 |
| Ofício Diário `awrv_2025_en` manhã | 273 | 721 | 2,6× | 49,1 | 19,8 |
| livros de oração | 452 | 990 | 2,2× | 31,5 | 15,7 |
| preferências `loc_2015` | 375 | 626 | 1,7× | 36,8 | 23,8 |
| v2 dia | 226 | 1.442 | 6,4× | 56,0 | 9,8 |
| v2 leituras | 220 | 1.472 | 6,7× | 54,1 | 9,8 |

### Sem cache (latência, uma requisição por vez, 20 datas)

| Endpoint | Rails média ms | Go média ms | Rails máx ms | Go máx ms |
|---|---:|---:|---:|---:|
| Ofício Diário `loc_2015` manhã | 145,6 | 21,1 | 334,6 | 29,6 |
| dia do calendário `loc_2015` | 203,8 | 34,0 | 429,6 | 58,7 |
| v2 dia `loc_2015` | 92,6 | 4,0 | 207,2 | 6,5 |

### Leitura dos números

* **O Go é mais rápido em todos os endpoints medidos**, com e sem cache. Sem
  cache, o cálculo litúrgico custa ~6–20× menos; é o número que importa para a
  primeira requisição de cada data e para os warmers.
* **Os caches foram decisivos.** Numa primeira medição, antes de o Go ter os
  caches que o Rails tem, o Go perdia onde o Rails servia do cache e o Go
  recalculava: dia do calendário (122 × 328 req/s), ano do calendário
  (183 × 245) e lecionário (314 × 361). Os caches do dia, do ofício base, da
  grade do calendário e do lecionário foram portados com as mesmas chaves,
  versões e TTLs do Rails ([EQUIVALENCE.md](EQUIVALENCE.md#7-caches)); isso
  também aproximou o comportamento (os dois recalculam nas mesmas edições).
  Uma otimização do resolvedor de celebrações (índice de datas ocupadas
  calculado uma vez por calendário) reduziu ~41% da CPU do Ofício Diário, e
  foi verificada com 13.334 requisições comparadas, 0 diferenças.
* **Onde a margem é menor** (Ofício Diário, preferências, livros): a resposta
  ainda é montada por requisição sobre o ofício base em cache (preferências do
  usuário, áudio, formatação), e em preferências e livros o custo é dominado por
  consultas ao banco, iguais para os dois.
* **Memória** (RSS medido logo após o benchmark): Go, um processo, **72 MB**;
  Rails na configuração de produção, **707 MB** (processo mestre do Puma 182 MB
  + 2 workers de 271 e 255 MB).

### Ressalvas

* Máquina compartilhada de 4 vCPUs: servidor, banco, Redis e gerador de carga
  disputam a mesma CPU. Em produção os números absolutos serão outros; a
  proporção entre os dois é o que esta medição sustenta.
* Mede leitura. Escritas não foram medidas.
* Uma rodada de 10 s por endpoint e servidor. Entre rodadas diferentes, os
  números do mesmo servidor variaram ~5–15%, bem abaixo das diferenças acima.
