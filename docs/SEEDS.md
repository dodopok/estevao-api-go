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
`rails db:seed` num banco vazio e `estevao-seed export` desse banco. O que sai é
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

Um livro novo, ou uma correção de texto, é uma edição nesses arquivos. Para
refazer tudo a partir das fontes Rails (depois de mudar os seeds Ruby):
`tools/seed-export.sh` (~12 min).

## Carga

```bash
psql "$DATABASE_URL" -f db/schema.sql     # banco vazio
estevao-seed load                          # ~4 s (o db:seed do Rails leva ~12 min)
```

O loader roda numa transação e **recusa** um banco em que qualquer tabela de
referência já tenha linhas. O `db:seed` do Rails apaga e recria os dados de
referência; num banco com usuários isso é destrutivo (e parcial: `PrayerBook.
destroy_all` falha em silêncio para livros com salmos), então o Go não
reproduz esse modo.

## Verificação

`tools/seed-verify.sh` cria um banco vazio a partir de `db/schema.sql`, carrega
`seeds/`, exporta de novo e compara com `seeds/` (diff vazio). Além disso,
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
