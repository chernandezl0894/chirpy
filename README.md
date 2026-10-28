# chirpy

## Usage

### Database instalation

```bash
sudo apt update
sudo apt install postgresql postgresql-contrib
psql --version
sudo passwd postgres
sudo -u postgres psql

CREATE DATABASE chirpy;
\c chirpy -- Change to chirpy database.

ALTER USER postgres WITH PASSWORD 'postgres'; -- Set the user password.

SELECT version();
```

### Migrations

**Instalation**

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
goose -version
```

inside of `sql/schema` run the following commands:

```bash
goose postgres "postgres://postgres:postgres@127.0.0.1:5432/chirpy" up
goose postgres "postgres://postgres:postgres@127.0.0.1:5432/chirpy" down
```

### Generate Queries and Go Struct (sqlc)

**Instalation**

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc version
```

- Create a `sqlc.yaml` file:

```yaml
version: "2"
sql:
  - schema: "sql/schema"
    queries: "sql/queries"
    engine: "postgresql"
    gen:
      go:
        out: "internal/database"
```

In the root of the project run the following commands:

```bash
sqlc generate
```

Before that you should create your queries inside of `sql/queries/*.sql`, it will create a `internal/database` folder.

### PostgreSQL comands

```bash
psql "postgres://postgres:postgres@127.0.0.1:5432/chirpy"
\dt - show all tables
\q - exit psql shell
```

### Compile and run

```bash
go build
./chirpy
```

## Authentication

### Package

`go get github.com/alexedwards/argon2id`
