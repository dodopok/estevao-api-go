# Banco de dados

**Resumo:** o Go usa o banco de produção como ele está e passa a ser o dono do
schema (`estevao db`). O histórico de migrações continua na mesma tabela, e
nenhum dado é convertido. Enquanto o Rails existir, os dois podem rodar contra o
mesmo banco, e voltar ao Rails é trocar o processo; a partir da primeira
migração do Go, esse retorno depende de a migração ser compatível com o código
Rails (seção 1, regras).

## 1. Schema e migrações

O Go é o dono do schema. O banco de produção continua o mesmo: nenhuma tabela é
recriada, nenhum dado é convertido.

* **Mesmo formato do Active Record.** O Go lê e escreve as mesmas tabelas,
  colunas e sequências:
  * `jsonb` com o mesmo shape e enums como os inteiros do Rails;
  * timestamps sem fuso, em UTC, com microssegundos;
  * `updated_at` alterado só quando o Active Record alteraria;
  * os mesmos `touch`, contadores e remoções em cascata (`dependent:`).
* **Um só histórico.** As migrações ficam em `db/migrations/*.sql` e são
  registradas na mesma tabela `schema_migrations` que o Rails criou, com versões
  no mesmo formato (`AAAAMMDDhhmmss`). O banco de produção mantém as 131 versões
  do Rails, até `20260922160000` (a *baseline*), e as do Go vêm depois.
* **Um migrador por vez.** O migrador usa o mesmo *advisory lock* do
  `ActiveRecord::Migrator` (`2053462845 * crc32(current_database)`). Um migrador
  Go e um Rails nunca rodam ao mesmo tempo no mesmo banco: o segundo falha, como
  o `ConcurrentMigrationError` do Rails.
* **`db/schema.sql`** é o schema completo depois de todas as migrações, com as
  versões aplicadas (o equivalente do `schema.rb`). Ele vai embutido no binário:
  cada build carrega exatamente o schema contra o qual foi compilado.

### Comandos (`estevao db`)

| Comando | O que faz |
|---|---|
| `prepare` | Equivalente ao `rails db:prepare`, e é o que o deploy roda. Cria o banco se não existir. Num banco sem tabelas, carrega `schema.sql` e o `seeds/`. Num banco existente, aplica as migrações pendentes. Recusa um banco com tabelas mas sem a baseline |
| `migrate` | Aplica as pendentes, cada uma numa transação junto com a sua linha em `schema_migrations`. Uma falha para ali e não deixa rastro |
| `status` | Lista as migrações do Go (aplicada/pendente) e conta o histórico Rails |
| `rollback [-steps N]` | Reverte as últimas N migrações do Go pela seção `down`. As do Rails e as sem `down` são irreversíveis |
| `new <nome>` | Cria `db/migrations/<versão>_<nome>.sql` |
| `dump` | Reescreve `db/schema.sql` a partir do banco local. Precisa do `pg_dump` 16; é ferramenta de desenvolvimento |

### Criar uma migração

```bash
estevao db new add_nickname_to_users      # editar as seções up/down
estevao db migrate                         # banco local (DATABASE_URL)
estevao db dump                            # atualiza db/schema.sql
tools/schema-check.sh                      # o mesmo teste do CI
```

O formato é o do [dbmate](https://github.com/amacneil/dbmate), que usa uma
tabela `schema_migrations(version varchar)` compatível. O dbmate serve de
ferramenta de emergência sobre os mesmos arquivos.

```sql
-- migrate:up
ALTER TABLE users ADD COLUMN nickname varchar;

-- migrate:down
ALTER TABLE users DROP COLUMN nickname;
```

### Regras para migrar produção sem parar o serviço

Durante o deploy, a versão anterior continua servindo enquanto a migração roda.
Toda migração precisa funcionar com as duas versões do código (*expand/contract*):

* **Adicionar** coluna (nula ou com default), tabela ou índice é seguro. O Go
  lista as colunas que lê, então uma coluna nova não quebra a versão anterior.
* **Renomear ou remover** coluna leva dois deploys: primeiro o código para de
  usá-la, depois uma migração a remove.
* **Índices** em tabelas grandes: `CREATE INDEX CONCURRENTLY` numa migração
  própria com `-- migrate:up transaction:false`, com um único comando.
* **Tempo de espera:** cada migração roda com `lock_timeout` de 5 s
  (`MIGRATION_LOCK_TIMEOUT`) e sem `statement_timeout`. Uma migração presa
  atrás de uma query longa falha, e o deploy para, em vez de enfileirar todas as
  requisições atrás dela.
* **Dados:** mudar dados de referência (textos, coletas, leituras) não é
  migração, é `estevao seed sync` ([SEEDS.md](SEEDS.md)). Migração de dados de
  usuário é SQL na migração, em lotes quando a tabela for grande.

### O CI verifica

* os testes do migrador contra bancos descartáveis;
* que `db/schema.sql` é exatamente o schema que as migrações produzem
  (`tools/schema-check.sh`: uma migração commitada sem `estevao db dump`
  aparece como diferença);
* que o seeder reconstrói `seeds/` (`tools/seed-verify.sh`).

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
