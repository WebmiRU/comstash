Atlas migration layout:

- `migrations/sqlite` for SQLite-specific SQL migrations
- `migrations/postgres` for PostgreSQL-specific SQL migrations

Typical workflow:

```bash
# Install Atlas CLI.
curl -sSf https://atlasgo.sh | sh

# Generate atlas.sum after editing migration files.
atlas migrate hash --dir "file://migrations/sqlite"
atlas migrate hash --dir "file://migrations/postgres"

# Apply migrations.
atlas migrate apply --env sqlite
atlas migrate apply --env postgres
```

Useful overrides:

```bash
atlas migrate apply --env sqlite --var sqlite_url="sqlite://db?_fk=1"
atlas migrate apply --env postgres --var postgres_url="postgres://user:pass@127.0.0.1:5432/comstash?search_path=public&sslmode=disable"
```
