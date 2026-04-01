# Comstash - Private Composer Proxy/Cache

A private Composer proxy/cache for [Kubernetes](https://kubernetes.io), [Nomad](https://www.hashicorp.com/en/products/nomad), local development, and similar use cases.

## Changelog
- v0.1: first release

## Roadmap
- Add an API and admin panel
- Finish PostgreSQL support
- Add cache expiration for packages
- Add Kubernetes manifests and/or a Helm chart
- Add Nomad configuration
- Add Git-based package support (for packages without a dist URL and source installs)
- Add Nginx to serve static ZIP archives for slow clients

## Database

Schema migrations are managed with Atlas.

Install Atlas:

```bash
curl -sSf https://atlasgo.sh | sh
```

Apply SQLite migrations:

```bash
atlas migrate hash --dir "file://migrations/sqlite"
atlas migrate apply --env sqlite
```

Apply PostgreSQL migrations:

```bash
atlas migrate hash --dir "file://migrations/postgres"
atlas migrate apply --env postgres
```
