# Estratégia de banco de dados (não destrutiva)

**Resumo:** o Go usa o banco de produção como ele está. Não há migração de
dados, não há DDL executado pelo Go, e Rails e Go podem rodar ao mesmo tempo
contra o mesmo banco. Voltar ao Rails é trocar o processo; os dados continuam
válidos para os dois.

## 1. Mesmo schema, mesmo dono

* O Go lê e escreve **as mesmas tabelas, colunas e sequências** que o Active
  Record, no mesmo formato: `jsonb` com o mesmo shape, enums como os inteiros do
  Rails, timestamps sem fuso em UTC com microssegundos, `updated_at` alterado só
  quando o Active Record alteraria, os mesmos `touch`, contadores e remoções em
  cascata (`dependent:`).
* **As migrações continuam sendo do Rails.** O Go não executa `CREATE`/`ALTER`/
  `DROP` em nenhum momento, e não precisa de nenhuma migração nova: todo o port
  foi feito contra o schema atual (`db/schema.rb`, 131 migrações).
* `db/schema.sql` é só uma fotografia desse schema (gerada por
  `tools/schema-dump.sh` a partir de um `db:schema:load` do Rails), usada para
  criar bancos **novos e locais** (desenvolvimento, CI, seed). Ela inclui as
  versões em `schema_migrations`, então um banco criado por ela é reconhecido
  pelo Rails como atualizado.
* Enquanto o Rails existir, qualquer mudança de schema entra como migração Rails
  e é aplicada pelo `db:prepare` do deploy Rails (ou pelo `preDeployCommand`
  atual); depois, `tools/schema-dump.sh` atualiza `db/schema.sql`. Depois de
  desligar o Rails em definitivo, a recomendação é adotar uma ferramenta de
  migração em Go (ex.: `golang-migrate`) partindo de `db/schema.sql` como linha de
  base e mantendo `schema_migrations` — nunca reaplicar o schema em cima de um
  banco existente.

## 2. Estado compartilhado fora das tabelas de domínio

| Estado | Como os dois lados convivem |
|---|---|
| Fila (`solid_queue_*`) | O Go escreve jobs exatamente como o Solid Queue 1.3/ActiveJob 8.1; cada lado executa jobs enfileirados pelo outro (verificado com `DIFF_CROSS_JOBS=1`). Os dois workers podem até rodar juntos: o `claim` usa `FOR UPDATE SKIP LOCKED` como o Solid Queue |
| Agenda recorrente | mesmas linhas em `solid_queue_recurring_tasks`/`_executions`; a chave única `(task_key, run_at)` impede execução dupla se os dois agendadores estiverem ligados |
| Active Storage | mesmas tabelas `active_storage_*`, mesmas chaves no bucket, mesmas URLs assinadas |
| Rate limit (Rack::Attack) e contadores de uso de API key | mesmas chaves no mesmo Redis (`estevao_api_v8:`), valores inteiros — um único orçamento para os dois lados |
| Caches | separados: o Rails guarda Marshal, o Go guarda JSON sob `go/`. Quando o Go escreve dados que o Rails mantém em cache sem versão, ele apaga a entrada do Rails como o Rails apagaria (status de conclusão do ofício; autenticação e multiplicadores de API key). Caches versionados por `updated_at` se invalidam sozinhos porque o Go atualiza `updated_at` como o Rails |

## 3. O que nunca é feito

* Nenhum `TRUNCATE`, `DELETE` em massa ou reconstrução de dados de referência. O
  seeder Go (`estevao-seed load`) **recusa** qualquer banco em que uma das tabelas
  de referência já tenha linhas; ele só preenche bancos vazios.
* Nenhum script aponta para um banco que não seja local: `bootstrap-db.sh` e os
  scripts de `tools/` verificam isso ou usam bancos temporários próprios.

## 4. Verificações recomendadas antes do corte

Todas só leitura:

```sql
-- versão do schema igual à esperada pelo Go
SELECT max(version) FROM schema_migrations;          -- 20260922160000
-- nenhum job com classe desconhecida pelo worker Go
SELECT class_name, count(*) FROM solid_queue_jobs WHERE finished_at IS NULL GROUP BY 1;
-- extensões presentes
SELECT extname FROM pg_extension;                     -- amcheck, pageinspect, plpgsql
```

`GenerateLiturgicalAudioJob` é a única classe do Rails sem handler no Go (só
roda inline pela rake `audio:generate`); se aparecer pendente na fila, deixe o
worker Rails ativo até ela esvaziar.
