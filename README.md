# DBTestify: test helper tool for the system that uses database

![icon](https://raw.githubusercontent.com/shibukawa/dbtestify/refs/heads/main/docs/dbtestify_icon.png)

[![Go Reference](https://pkg.go.dev/badge/github.com/shibukawa/dbtestify.svg)](https://pkg.go.dev/github.com/shibukawa/dbtestify)
![GitHub Workflow Status](https://img.shields.io/github/workflow/status/shibukawa/dbtestify/CI)

## Install

### Binary

Binary is available for Linux, macOS, and Windows. You can download the latest release from [GitHub Releases](https://github.com/shibukawa/dbtestify/releases).

### Container

Install:

```shell
$ docker pull ghcr.io/shibukawa/dbtestify:latest 
```

Execute:
:

```shell
# Execute CLI command
$ docker run --rm -it -e "DBTESTIFY_CONN=(url)" ghcr.io/shibukawa/dbtestify:latest dbtestify [command] [args...]

# Publish API Server
$ docker run --rm -it -p 8080:8000-e "DBTESTIFY_CONN=(url)" ghcr.io/shibukawa/dbtestify:latest dbtestify http -p 8000
```

### For Go users

Install:

```shell
$ go get -tool github.com/shibukawa/dbtestify/cmd/dbtestify
```

Execute:

```shell
$ go tool dbtestify [command] [args...]
```

## Usage

### Database Connection

It supports PostgreSQL, MySQL, SQLite. Set the following use the following connection string to `DBTESTIFY_CONN` environment variable (or `--db` flag in CLI).

* PostgreSQL: [detail](https://github.com/jackc/pgx)
  * `postgres://user:pass@localhost:5432/dbname?sslmode=disable`
* MySQL: [detail](https://github.com/go-sql-driver/mysql)
  * `mysql://root:pass@tcp(localhost:3306)/foo?tls=skip-verify`
* SQLite: [detail](https://github.com/mattn/go-sqlite3)
  * `sqlite://file:dbfilename.db`

Then prepare the data set file in YAML format. This file is used for seeding the database and asserting the data in the database.

```yaml
member:
- { id: 1, name: Frank, email: frank@example.com }
- { id: 2, name: Grace, email: grace@example.com }
- { id: 3, name: Heidi, email: heidi@example.com }
- { id: 4, name: Ivan }

belonging:
- { member_id: 1, group_id: 1 }
- { member_id: 2, group_id: 1 }
- { member_id: 3, group_id: 2 }
- { member_id: 4, group_id: 2 }

group:
- { id: 1, name: "Group A" }
- { id: 2, name: "Group B" }
```

You can seed/assert the data set from CLI, HTTP API, Go library

### CLI

Use the following command to seed the data set into the database:

```shell
$ dbtestify seed testdata/users.yaml
$ dbtestify assert testdata/users.yaml
```

### HTTP API

`http` subcommand launches a HTTP server.

```shell
$ dbtestify http -p 8000 ../testdata
dbtestify API server

        GET  http://localhost:8000/api/list                    : Show data set file list
        POST http://localhost:8000/api/seed/{data set path}    : Seed database content with the specified data set
        GET  http://localhost:8000/api/assert/{data set path}  : Assert database content with the specified data set
        start receiving at :8000
```

```shell
$ curl -X POST http://localhost:8000/api/seed/users.yaml
$ curl http://localhost:8000/api/assert/users.yaml
```

```ts
const DBTESTIFY_URL = 'http://localhost:8000/api';

test.beforeEach(async ({ request }) => {
    await request.post(`${DBTESTIFY_URL}/api/seed/users.yaml`)
});

test("add user", async ({ page, request }) => {
    await page.getByRole("button", { name: "Add User" }).click();
    await request.get(`${DBTESTIFY_URL}/api/assert/users.yaml`);
});
```

### Go Unit Tests

`github.com/shibukawa/dbtestify/assertdb` packages provides a helper for Go unit tests. Just calling `assertdb.SeedDataSet` and `assertdb.AssertDB` functions in your test code.

```go
import (
	"embed"
	"os"
	"testing"

	"github.com/shibukawa/dbtestify/assertdb"
)

//go:embed dataset/*
var dataSet embed.FS

const dbtestifyConn = "sqlite://file:database.db"

func TestUsage(t *testing.T) {
    assertdb.SeedDataSet(t, dbtestifyConn, dataSet, "initial.yaml", nil)

    // some logic that modifies the database

    assertdb.AssertDB(t, dbtestifyConn, dataSet, "expect.yaml", nil)
```

## Data Set Reference

Data set definition is a key feature of dbtestify. Data set is defined in YAML format. Basic structure is like this:

```yaml
member:
- { id: 1, name: Frank, email: frank@example.com }
- { id: 2, name: Grace, email: grace@example.com }
- { id: 3, name: Heidi, email: heidi@example.com }
- { id: 4, name: Ivan }

belonging:
- { member_id: 1, group_id: 1 }
- { member_id: 2, group_id: 1 }
- { member_id: 3, group_id: 2 }
- { member_id: 4, group_id: 2 }

group:
- { id: 1, name: "Group A" }
- { id: 2, name: "Group B" }
```

Data set is used in two purposes. There are special options:

* For seeding of test data in unit tests
  This data set is imported into the database before running tests.
* For assertion the data in database
  This data set is compared with the data in the actual database.

### Data Set for Seeding

There are several options for seeding

* `clear-insert`(default): Truncate the table then insert data.
* `insert`: Just insert data into the table.
* `upsert`: Insert data into the table, or update if the row already exists.
* `truncate`: Just truncate the table.
* `delete`: Delete rows in the table that matches the dataset's primary keys.

```yaml
_operations:
  user: clear-insert
  access_log_2025_*: truncate
  login_history: delete

user:
- { id: 10, name: Frank, email: frank@example.com }
- { id: 11, name: Grace, email: grace@example.com }
- { id: 12, name: Heidi, email: heidi@example.com }
- { id: 13, name: Ivan}

login_history
- { user_id: 10, time: 2024-12-14 }
```

### Data Set for Assertion

There are two options for matching rules.

* `exact`(default): The table should have exact same rows with the data set. But if the fields not in the data set is ignored (like system fields).
* `sub`: If the table has extra rows that are not in the data set, it is still valid.

And there are special field values for matching:

* `[null]`: It assumes the value is NULL. it is as same as `null`.
* `[notnull]`: It assumes the value is not NULL.
* `[any]`: It matches any value.
* `[currentdate, <duration>]`: It matches when the actual value is within the specified duration from the current time. The duration is optional and defaults to `1m` (Go duration format, e.g., `30s`, `2m`, `1h`). The actual value can be `time.Time` or a string parseable by RFC3339 or common SQL timestamp formats.
* `[regexp, <pattern>]`: It matches when the stringified actual value matches the given regular expression pattern.

```yaml
_match:
  user: exact
  access_log_2025_*: sub

user:
- { id: 10, name: Frank, email: null }

login_history
- { user_id: 10,. time: [notnull]}

audit_log:
- { id: 1, created_at: [currentdate, 2m], message: [regexp, "^User .+ logged in$"] }
```

### Tags

Each row can have tags. You can filter the rows by tags when loading:

```yaml
member:
- { id: 1, name: Frank, _tags: [admin, user] }
- { id: 2, name: Grace, _tags: [user] }
- { id: 3, name: Ivy, _tags: [ex_user] }
```

#### For CLI:

```shell
# you can use `-i`/`-e` as a shortcut 
$ dbtestify seed testdata/users.yaml --include-tag=user --exclude-tag=admin
$ dbtestify assert testdata/users.yaml --include-tag=user --exclude-tag=admin
```

#### For HTTP API:

You can use Form data or JSON to specify the tags. For curl command, use form data is easier than JSON. But if request from`fetch` or `request`(Playwright), you should feel using JSON is easier.

```shell
$ curl -X POST -d "i=user" -d "" http://localhost:8000/api/seed/users.yaml
$ curl -X POST -H "Content-Type: application/json" -d '{"include_tags": ["user"], "exclude_tags": ["admin"]}' http://localhost:8000/api/seed/users.yaml
$ curl -d "i=user" -d "" http://localhost:8000/api/assert/users.yaml
```

```yaml
$ dbtestify assert testdata/users.yaml --tags user
```


## License

* AGPL-3.0
