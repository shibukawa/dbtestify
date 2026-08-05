# DBTestify: データベースを使用するシステム向けテストヘルパーツール

![icon](https://raw.githubusercontent.com/shibukawa/dbtestify/refs/heads/main/docs/dbtestify_icon.png)

[![Go Reference](https://pkg.go.dev/badge/github.com/shibukawa/dbtestify.svg)](https://pkg.go.dev/github.com/shibukawa/dbtestify)
![GitHub Workflow Status](https://img.shields.io/github/workflow/status/shibukawa/dbtestify/CI)

## インストール

### バイナリ

バイナリはLinux, macOS, Windowsで利用可能です。最新のリリースは[GitHub Releases](https://github.com/shibukawa/dbtestify/releases)を参照してください。

### コンテナ

インストール:

```shell
$ docker pull ghcr.io/shibukawa/dbtestify:latest 
```

実行:

```shell
# CLIコマンドを実行
$ docker run --rm -it -e "DBTESTIFY_CONN=(url)" ghcr.io/shibukawa/dbtestify:latest dbtestify [command] [args...]

# APIサーバーを公開
$ docker run --rm -it -p 8080:8000-e "DBTESTIFY_CONN=(url)" ghcr.io/shibukawa/dbtestify:latest dbtestify http -p 8000
```

### Go ユーザー向け

インストール:

```shell
$ go get -tool github.com/shibukawa/dbtestify/cmd/dbtestify
```

実行:

```shell
$ go tool dbtestify [command] [args...]
```

## 使用方法

### データベース接続

PostgreSQL、MySQL、SQLiteをサポートしています。以下の接続文字列を `DBTESTIFY_CONN` 環境変数（またはCLIの `--db` フラグ）に設定してください。

* PostgreSQL: [詳細](https://github.com/jackc/pgx)
  * `postgres://user:pass@localhost:5432/dbname?sslmode=disable`
* MySQL: [詳細](https://github.com/go-sql-driver/mysql)
  * `mysql://root:pass@tcp(localhost:3306)/foo?tls=skip-verify`
* SQLite: [詳細](https://github.com/mattn/go-sqlite3)
  * `sqlite://file:dbfilename.db`

次に、YAML形式でデータセットファイルを準備します。このファイルはデータベースのデータ投入(シード化)と、データベース内のデータのアサーションに使用されます。

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

データセットのデータ投入/アサーションは、CLI、HTTP API、Goライブラリから行うことができます。

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

#### アプリケーションへの組み込み

別プロセスのサーバーからは、インメモリのSQLiteには接続できません。ファイルベースのSQLiteでも、アプリケーション自身のライターと書き込みが競合します。`httpapi.NewHandler` は、呼び出し側が持っているリソースの上に同じ3つのエンドポイントを構築し、`http.Handler` として返します。テスト対象のアプリケーション自身が、ビルドタグで保護した上でループバックに限定して公開する、という使い方を想定しています。

```go
import (
    "net/http"

    "github.com/shibukawa/dbtestify"
    "github.com/shibukawa/dbtestify/httpapi"
)

h, err := httpapi.NewHandler(httpapi.Config{
    // リクエストごとに1回呼ばれます。毎回同じコネクタを返しても構いません。
    // トランザクションベースのコネクタを返せば、データ投入を呼び出し側が
    // 管理するトランザクションの中に閉じ込められます。
    Connector: func(ctx context.Context) (dbtestify.DBConnector, error) {
        return dbtestify.NewDBConnectorFromDB(pool, dbtestify.PostgresDialect)
    },
    DataSets:   os.DirFS("testdata"),  // リクエストのパスはこのルート内で解決されます
    AlwaysJSON: true,                  // Acceptヘッダーによらずjsonで応答します
})

mux.Handle("/_e2e/api/", http.StripPrefix("/_e2e", h))
```

パス、リクエストオプション、`SeedResponse`/`AssertResponse` のレスポンス、アサーション失敗時の400ステータスは、いずれもCLIのサーバーと同じです。`dbtestify http` 向けに書かれたテストは、ベースURLを変えるだけでそのまま動きます。JSONで応答する場合、エラーも `{"error": "..."}` の形式で返します。

`httpapi` パッケージは `database/sql` のドライバーを一切登録しません。importしてもpgxやgo-sql-driver/mysql、cgoが必要なmattn/go-sqlite3がリンクされないため、`CGO_ENABLED=0` のビルドからも利用できます。

> **破壊的変更:** `httpapi.Start` は `github.com/shibukawa/dbtestify/httpapi/serve` に移動しました。ドライバーのimportもこちらに移っています。シグネチャは変わりません。またサーバーは、リクエストごとではなく起動時に1回だけ接続を開くようになりました。

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

* ユニットテストでのテストデータのデータ投入
  このデータセットは、テスト実行前にデータベースにインポートされます。
* データベース内のデータのアサーション
  このデータセットは、実際のデータベース内のデータと比較されます。

### データ投入用データセット

データ投入時の処理には複数のオプションがあります：

* `clear-insert`（デフォルト）: テーブルをトランケートしてからデータを挿入。
* `insert`: テーブルにデータを挿入するだけ。
* `upsert`: テーブルにデータを挿入、行が既に存在する場合は更新。
* `truncate`: テーブルをトランケートするだけ。
* `delete`: データセットの主キーにマッチするテーブル内の行を削除。

```yaml
_operation:
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
* `[currentdate, <duration>]`: 実値が現在時刻から指定の許容差（Goのduration形式。省略時は `1m`）の範囲内であればマッチします。実値は `time.Time` もしくは RFC3339 / 一般的なSQLタイムスタンプ形式の文字列を想定します。
* `[regexp, <pattern>]`: 実値を文字列化して、与えた正規表現パターンにマッチすれば一致とみなします。

```yaml
_match:
  user: exact
  access_log_2025_*: sub

user:
- { id: 10, name: Frank, email: null }

login_history
- { user_id: 10, time: [notnull]}

audit_log:
- { id: 1, created_at: [currentdate, 2m], message: [regexp, "^User .+ logged in$"] }
```

### タグ

各行にタグを付けることができます。ロード時にタグで行をフィルタリングできます：

```yaml
member:
- { id: 1, name: Frank, _tag: [admin, user] }
- { id: 2, name: Grace, _tag: [user] }
- { id: 3, name: Ivy, _tag: [ex_user] }
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

以下のいずれかを、利用者が選択できるデュアルライセンスです。

* GNU Affero General Public License バージョン3以降（[LICENSE-AGPL-3.0.md](LICENSE-AGPL-3.0.md)）
* Apache License バージョン2.0（[LICENSE-APACHE-2.0.txt](LICENSE-APACHE-2.0.txt)）

`SPDX-License-Identifier: AGPL-3.0-or-later OR Apache-2.0`

HTTP APIを自分のアプリケーションから配信する場合を含め、ライブラリとしての組み込みはどちらのライセンスでも認められています。AGPLのネットワーク利用条項が適さないプロジェクトでは、Apache-2.0を選んでください。詳細は [LICENSE.md](LICENSE.md) を参照してください。