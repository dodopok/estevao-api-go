# Seeds em Go

## O problema

No Rails, cada Livro de Oração tem seu próprio jeito de semear
(`db/seeds/prayer_books/<livro>/seed.rb`): arquivos Ruby com hashes, JSON, CSV
lidos por importadores, dados copiados de outro livro (`loc_2019_es` copia o
lecionário de `loc_2019_en`), leituras derivadas do calendário
(`loc_1991_pt`), além de catálogos globais (versões bíblicas, saltérios,
cores, estações, regras de vida, música de fundo). São ~400 arquivos em vários
formatos — portar cada fonte seria portar vinte importadores diferentes.

## A solução: um dataset canônico

Em vez de portar as fontes, o resultado delas foi exportado **uma vez**:
`rails db:seed` num banco vazio e `estevao seed export` desse banco. O que sai é
`seeds/`, um único formato para todos os livros:

```
seeds/
  manifest.json                     ordem de inserção, lacunas de id, sequências
  liturgical_colors.json  liturgical_seasons.json  bible_versions.json
  bible_texts.json                  os 5 saltérios (Coverdale ×3, IEAB ×2)
  prayer_books.json  users.json  life_rules.json  life_rule_steps.json  journals.json
  background_*.json                 catálogo de música de fundo
  prayer_books/<livro>/
    celebrations.json  collects.json  lectionary_readings.json  liturgical_texts.json
    preference_categories.json  preference_definitions.json  psalms.json  psalm_cycles.json
```

Regras do formato:

* Um arquivo JSON por tabela, uma linha por registro, na ordem em que o Rails
  inseriu (a ordem dos ids, da qual dependem vários `ORDER BY id`).
* Sem `id`, `created_at`, `updated_at`. Chaves estrangeiras viram a **chave
  natural** do registro apontado: `"prayer_book": "loc_2015"`, `"celebration":
  "Páscoa"` (ou `["loc_2019_en", "Andrew the Apostle"]` quando aponta para
  outro livro), `"season": "Advento"`, `"category": "daily_office"`, `"user":
  "system@estevao.app"`, `"track": "<slug>"`.
* Enums do Rails por nome (`"celebration_type": "principal_feast"`).
* Datas que o Rails calcula a partir de `Date.today` (as anotações de exemplo)
  são gravadas como `"@today"`, `"@today-1"` e resolvidas no dia da carga.
* `manifest.json` guarda a ordem de inserção entre arquivos, os ids que o Rails
  consumiu sem manter linha (registros criados e apagados durante o seed) e o
  valor final de cada sequência — por isso um banco semeado pelo Go tem **os
  mesmos ids** de um semeado pelo Rails.

**`seeds/` é a fonte do conteúdo.** Um livro novo, uma correção de texto, uma
coleta ou uma leitura é uma edição nesses arquivos, revisada em PR como
qualquer código. Os seeds Ruby do Rails ficaram como histórico: o dataset foi
exportado deles uma vez e não é mais regenerado a partir deles.

## Banco novo: `estevao db prepare`

```bash
estevao db prepare     # cria o banco, carrega db/schema.sql e seeds/ (~5 s)
```

É o `rails db:prepare`. Num banco sem tabelas, ele carrega o schema e depois
`estevao seed load`. Esse loader roda numa transação e **recusa** um banco em
que qualquer tabela de referência já tenha linhas.

O `db:seed` do Rails apagava e recriava os dados de referência. Num banco com
usuários isso é destrutivo, e parcial: `PrayerBook.destroy_all` falha em
silêncio para livros com salmos. O Go não tem esse modo.

## Banco existente: `estevao seed sync`

O `sync` substitui as tarefas incrementais do Rails:
* `prayer_books:seed[código]` e `prayer_books:setup_dwdo`;
* `liturgical_texts:sync_catalog`, `import:collects`, `psalters:seed`;
* `background_music:seed`.

```bash
estevao seed sync                      # só relata o que mudaria
estevao seed sync -book loc_2015       # só um livro (a linha em prayer_books e o seu diretório)
estevao seed sync -apply               # escreve
```

Como compara:

* **Linhas com identidade natural** são casadas por ela. Para cada tabela:

  | Tabela | Identidade |
  |---|---|
  | `celebrations` | livro + `name` |
  | `liturgical_texts` | livro + `slug` |
  | `lectionary_readings` | livro + data + ciclo + ofício + tipo + variante + celebração |
  | `psalms` | livro + `number` |
  | `psalm_cycles` | livro + chave do ciclo |
  | `preference_*` | livro + `key` |
  | `bible_texts` | tradução + livro + capítulo + versículo |
  | `prayer_books`, `bible_versions` | `code` |
  | trilhas de música | `slug` |

  Uma linha alterada é atualizada **no lugar**: o id se mantém, porque dados de
  usuário e clientes podem guardá-lo. Uma linha nova é inserida.
* **Coletas** não têm identidade: há alternativas para o mesmo dia. O grupo
  (livro + celebração + estação + domingo + estilo de linguagem) é comparado
  como lista ordenada e, se mudou, é substituído inteiro na ordem do dataset.
  Nenhum dado de usuário aponta para coletas.
* **Nunca apaga.** Linhas que só existem no banco são listadas e ficam. Há dados
  de usuário que apontam para conteúdo, alguns com `ON DELETE CASCADE`
  (`user_audio_usages` → `liturgical_texts`); remover conteúdo é uma migração
  revisada.
* **Colunas de processo não são sobrescritas** numa linha existente:
  * o áudio legado de `liturgical_texts` (`audio_url`, `audio_urls`,
    `audio_generation_status`);
  * `published_at` e a duração medida de `background_tracks`.
* **Fora do sync:**
  * `feature_flags` e `background_track_assets` (estado de produção);
  * os usuários de sistema, as regras de vida embutidas e as anotações de
    exemplo, criados uma vez pelo `load`;
  * as Bíblias completas (`estevao bible`).

Tudo roda numa transação, com `lock_timeout` de 5 s. No fim, o `updated_at` de
cada livro cujo conteúdo mudou é tocado. Isso invalida todos os caches versionados
pelo livro: os do Redis e os em memória de cada instância. Os demais livros não
são tocados.

Antes de aplicar, o `sync` recusa um dataset inconsistente:
* uma linha num diretório de livro que aponta para outro livro;
* duas linhas com a mesma identidade;
* um campo que não é coluna da tabela.

### Primeiro uso em produção

O `seeds/` saiu do `rails db:seed`, e a produção pode ter divergido dele por
edições feitas por rake ou console ao longo do tempo. Antes do primeiro
`-apply`:

1. Rode `estevao seed sync` **sem `-apply`** contra produção. Só lê.
2. Para cada diferença, decida qual lado está certo. Se for a produção, edite
   `seeds/` para refletir o valor dela. Se for o dataset, deixe para o `-apply`.
3. Repita até o relatório mostrar só o que deve mudar. Então aplique.

Depois disso, conteúdo passa a seguir o mesmo caminho do código: PR → CI →
deploy → `estevao seed sync -apply`. Esse passo pode entrar no
`preDeployCommand`, depois do `estevao db prepare`, quando houver confiança no
fluxo.

## Verificação

`tools/seed-verify.sh` cria um banco vazio com `estevao db prepare`, exporta de
novo e compara com `seeds/` (diff vazio). O teste do `sync`
(`internal/seed/sync_test.go`, no CI) carrega o dataset, exige zero diferenças,
altera uma cópia (texto, celebração nova, coleta nova no grupo) e verifica:
* o plano;
* que o relatório sem `-apply` não escreve;
* que o id se mantém e a coluna de processo não é tocada;
* que o livro é tocado;
* que uma segunda passada não encontra nada. Além disso,
comparei tabela por tabela o banco semeado pelo Go com o semeado pelo Rails
(`to_jsonb` de cada linha, ordenado por id, menos os timestamps, e o valor de
cada sequência): **as 21 tabelas com dados são idênticas, ids incluídos**.

| Tabela | Linhas |
|---|---:|
| lectionary_readings | 37.946 |
| bible_texts (saltérios) | 12.436 |
| liturgical_texts | 6.566 |
| celebrations | 4.085 |
| collects | 3.312 |
| psalm_cycles | 1.052 |
| preference_definitions | 501 |
| background_track_placements / _categories / tracks / categories | 247 / 200 / 82 / 12 |
| psalms, preference_categories, bible_versions, prayer_books | 157 / 105 / 33 / 24 |
| life_rule_steps, liturgical_colors, liturgical_seasons, users, life_rules, journals | 51 / 9 / 6 / 6 / 6 / 4 |

Fora do escopo do seed (como no Rails): os ~1 milhão de versículos das Bíblias
completas, que vêm das rakes `bible:*` a partir de arquivos SQLite externos, e a
importação de áudio da música de fundo (`BACKGROUND_MUSIC_IMPORT`).

## As fixtures refletem os seeds?

Quase, mas não inteiramente. Comparei `spec/fixtures/prayer_books/<livro>/*.json`
(geradas por `script/generate_prayer_book_fixtures.rb`) com o dataset, campo a
campo (`tools/seeds-vs-fixtures.py`):

* Os 24 livros têm fixtures, e das **53.724 linhas por livro** das 8 tabelas que
  as fixtures cobrem, **53.682 são idênticas**. As diferenças:
  * `loc_1979_en` — 8 coletas associadas só a uma estação (sem celebração e sem
    domingo) ficam fora das fixtures: o gerador pula coletas assim, e o formato
    não tem o campo `season`.
  * `loc_1987` — 4 coletas estão na "Quarta-feira Santa" nas fixtures e no
    "Sábado Santo" no seed: as fixtures (8/set) são anteriores à mudança do seed
    (21/set).
  * `loc_2019_es` — 30 leituras do seed apontam para celebrações de
    `loc_2019_en` (a cópia do lecionário leva a associação junto); o formato das
    fixtures só guarda o nome e, ao carregar, as associa ao próprio livro.
* Campos que existem no seed e não nas fixtures: `collects.season`,
  `psalm_cycles.notes`, `liturgical_texts.audio_*` e, em parte,
  `celebrations.latin_name`.
* Não estão nas fixtures de forma alguma: o catálogo de livros, as versões
  bíblicas, os 5 saltérios, cores, estações, regras de vida, usuários de
  sistema, anotações e o catálogo de música de fundo.

Por isso o dataset foi gerado a partir dos seeds, não das fixtures. Não foi
preciso portar as n fontes: rodar o seed Rails uma vez e exportar resolve o
problema com um único caminho mecânico, e o `seed-verify` prova o resultado.
