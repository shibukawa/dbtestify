# DBTestify: データベースを使用するシステム向けテストヘルパーツール

[![GitHub License](https://img.shields.io/github/license/shibukawa/dbtestify)](
[![Go Reference](https://pkg.go.dev/badge/github.com/shibukawa/dbtestify.svg)](https://pkg.go.dev/github.com/shibukawa/dbtestify)
[![GitHub Workflow Status](https://img.shields.io/github/workflow/status/shibukawa/dbtestify/CI)](

## インストール

### コンテナ

### Go ユーザー向け

## 使用方法

### データベース接続

PostgreSQL、MySQL、SQLiteをサポートしています。以下の接続文字列を `DBTESTIFY_CONN` 環境変数（またはCLIの `--db` フラグ）に設定してください。

* PostgreSQL: [詳細](https://github.com/jackc/pgx)
  * `postgres://user:pass@localhost:5432/dbname?sslmode=disable`
* MySQL: [詳細](https://github.com/go-sql-driver/mysql)
  * `mysql://root:pass@tcp(localhost:3306)/foo?tls=skip-verify`
* SQLite: [詳細](https://github.com/mattn/go-sqlite3)
  * `sqlite://file:dbfilename.db`

次に、YAML形式でデータセットファイルを準備します。このファイルはデータベースのシード化と、データベース内のデータのアサーションに使用されます。

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

データセットのシード化/アサーションは、CLI、HTTP API、Goライブラリから行うことができます。

### CLI

以下のコマンドを使用してデータセットをデータベースにシード化します：

```shell
$ dbtestify seed testdata/users.yaml
$ dbtestify assert testdata/users.yaml
```

### HTTP API

`http` サブコマンドでHTTPサーバーを起動します。

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

test("ユーザー追加", async ({ page, request }) => {
    await page.getByRole("button", { name: "Add User" }).click();
    await request.get(`${DBTESTIFY_URL}/api/assert/users.yaml`);
});
```

### Go ユニットテスト

`github.com/shibukawa/dbtestify/assertdb` パッケージは、Goユニットテスト用のヘルパーを提供します。テストコード内で `assertdb.SeedDataSet` と `assertdb.AssertDB` 関数を呼び出すだけです。

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

    // データベースを変更するロジック

    assertdb.AssertDB(t, dbtestifyConn, dataSet, "expect.yaml", nil)
```

## データセットリファレンス

データセット定義は dbtestify の主要機能です。データセットはYAML形式で定義されます。基本構造は以下のとおりです：

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

データセットは2つの目的で使用されます。特別なオプションは以下のとおりです：

* ユニットテストでのテストデータのシード化
  このデータセットは、テスト実行前にデータベースにインポートされます。
* データベース内のデータのアサーション
  このデータセットは、実際のデータベース内のデータと比較されます。

### シード化用データセット

シード化には複数のオプションがあります：

* `clear-insert`（デフォルト）: テーブルをトランケートしてからデータを挿入。
* `insert`: テーブルにデータを挿入するだけ。
* `upsert`: テーブルにデータを挿入、行が既に存在する場合は更新。
* `truncate`: テーブルをトランケートするだけ。
* `delete`: データセットの主キーにマッチするテーブル内の行を削除。

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

### アサーション用データセット

マッチングルールには2つのオプションがあります：

* `exact`（デフォルト）: テーブルはデータセットと完全に同じ行を持つ必要があります。ただし、データセットにないフィールド（システムフィールドなど）は無視されます。
* `sub`: テーブルがデータセットにない追加の行を持っていても有効です。

また、マッチング用の特別なフィールド値があります：

* `[null]`: 値がNULLであることを想定。`null` と同じです。
* `[notnull]`: 値がNULLではないことを想定。
* `[any]`: 任意の値にマッチ。

```yaml
_match:
  user: exact
  access_log_2025_*: sub

user:
- { id: 10, name: Frank, email: null }

login_history
- { user_id: 10, time: [notnull]}
```

### タグ

各行にタグを付けることができます。ロード時にタグで行をフィルタリングできます：

```yaml
member:
- { id: 1, name: Frank, _tags: [admin, user] }
- { id: 2, name: Grace, _tags: [user] }
- { id: 3, name: Ivy, _tags: [ex_user] }
```

#### CLI用：

```shell
# `-i`/`-e` をショートカットとして使用できます
$ dbtestify seed testdata/users.yaml --include-tag=user --exclude-tag=admin
$ dbtestify assert testdata/users.yaml --include-tag=user --exclude-tag=admin
```

#### HTTP API用：

タグを指定するためにフォームデータまたはJSONを使用できます。curlコマンドでは、フォームデータの方がJSONより簡単です。しかし、`fetch` や `request`（Playwright）からリクエストする場合は、JSONの方が使いやすいでしょう。

```shell
$ curl -X POST -d "i=user" -d "" http://localhost:8000/api/seed/users.yaml
$ curl -X POST -H "Content-Type: application/json" -d '{"include_tags": ["user"], "exclude_tags": ["admin"]}' http://localhost:8000/api/seed/users.yaml
$ curl -d "i=user" -d "" http://localhost:8000/api/assert/users.yaml
```

```yaml
$ dbtestify assert testdata/users.yaml --tags user
```

## ライセンス

* AGPL-3.0