# Migrações

Um arquivo por migração, `<versão>_<nome>.sql`, criado por
`estevao db new <nome>`. A versão é um carimbo UTC `AAAAMMDDhhmmss`, no mesmo
formato das migrações Rails, e fica registrada na mesma tabela
`schema_migrations`. Assim o histórico continua no banco de produção sem
recomeçar.

```sql
-- migrate:up
ALTER TABLE users ADD COLUMN nickname varchar;

-- migrate:down
ALTER TABLE users DROP COLUMN nickname;
```

`-- migrate:up transaction:false` executa a migração fora de transação. É
obrigatório para `CREATE INDEX CONCURRENTLY`, e uma migração assim deve conter
um único comando.

Regras e procedimento: [docs/DATABASE.md](../../docs/DATABASE.md).
