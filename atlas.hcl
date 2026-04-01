variable "sqlite_url" {
  type    = string
  default = "sqlite://db/db.sqlite?_fk=1"
}

variable "postgres_url" {
  type    = string
  default = "postgres://postgres:postgres@127.0.0.1:5432/comstash?search_path=public&sslmode=disable"
}

env "sqlite" {
  url = var.sqlite_url

  migration {
    dir = "file://migrations/sqlite"
  }
}

env "postgres" {
  url = var.postgres_url
  dev = "docker://postgres/16/dev?search_path=public"

  migration {
    dir = "file://migrations/postgres"
  }
}
