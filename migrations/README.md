# Database migrations (PostgreSQL)

Waverless metadata storage uses **PostgreSQL only**. Apply migrations with `psql` (or your migration runner) against the target database.

Fresh installs should use the full schema in [`scripts/init_postgres.sql`](../scripts/init_postgres.sql). The files in this directory are incremental migrations for existing databases; they mirror the PostgreSQL dialect used in `init_postgres.sql`.

```bash
psql "$DATABASE_URL" -f migrations/001_create_status_events_table.sql
# ... apply 002–005 in order as needed
```

Environment variables for connections are documented in the root [README](../README.md) and [User Guide](../docs/USER_GUIDE.md) (`DATABASE_URL`, `PG_*`, `POSTGRES_*`).
