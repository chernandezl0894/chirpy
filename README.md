# chirpy

## Usage

### Migrations

inside of `sql/schema` run the following commands:

```bash
goose postgres "postgres://postgres:postgres@127.0.0.1:5432/chirpy" up
goose postgres "postgres://postgres:postgres@127.0.0.1:5432/chirpy" down
```

### Generate Queries and Go Struct (sqlc)

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

