# Comstash - private proxy/cache composer repository
for [Kubernetes](https://kubernetes.io), [Nomad](https://www.hashicorp.com/en/products/nomad), local usage and other cases

## Changelist:
- v0.1 first release

## Database

Schema migrations are managed with Atlas.

Install Atlas:

```bash
curl -sSf https://atlasgo.sh | sh
```

Apply migrations:

```bash
atlas migrate hash --dir "file://migrations/sqlite"
atlas migrate apply --env sqlite
```

For PostgreSQL:

```bash
atlas migrate hash --dir "file://migrations/postgres"
atlas migrate apply --env postgres
```
