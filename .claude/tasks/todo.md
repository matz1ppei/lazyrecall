# Builder Task List — poc-anki-claude

Reference architecture: `.claude/plans/blueprint.md`

---

## Phase 1: プロジェクト初期化

- [x] `go mod init github.com/ippei/poc-anki-claude` を実行
- [x] 依存パッケージを追加:
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/lipgloss`
  - `modernc.org/sqlite`
  - `github.com/anthropics/anthropic-sdk-go`
- [x] `go mod tidy` を実行
- [x] ディレクトリ作成: `db/`, `srs/`, `ai/`, `tui/`, `importer/`
- [x] `.env.example` を作成 (blueprint.md 記載の内容)
- [x] `main.go` の骨格を作成 (後フェーズで実装、`fmt.Println("TODO")` で一旦OK)

---

## Phase 2: DB層

- [x] `db/schema.sql` を作成 — `cards` テーブルと `reviews` テーブルの DDL (blueprint.md 参照)
- [x] `db/db.go` を実装:
  - `Open(path string) (*sql.DB, error)` — SQLite接続 + `schema.sql` の自動マイグレーション (`PRAGMA foreign_keys = ON` を設定)
- [x] `db/cards.go` を実装:
  - `CreateCard(db, front, back, hint string) (id int64, err error)`
  - `GetCard(db, id int64) (Card, error)`
  - `ListCards(db) ([]Card, error)`
  - `DeleteCard(db, id int64) error`
  - `Card` struct: `ID, Front, Back, Hint, CreatedAt`
- [x] `db/reviews.go` を実装:
  - `GetOrCreateReview(db, cardID int64) (Review, error)` — 存在しなければ INSERT、存在すれば SELECT
  - `UpdateReview(db, r Review) error` — `interval, ease_factor, repetitions, last_rating, reviewed_at, due_date` を UPDATE
  - `ListDueCards(db) ([]CardWithReview, error)` — `due_date <= date('now')` の cards+reviews JOIN
  - `CardWithReview` struct: Card フィールド + Review フィールドをフラット化 or 埋め込み
  - `Review` struct: `ID, CardID, DueDate, Interval, EaseFactor, Repetitions, LastRating, ReviewedAt`
- [x] `db/` の関数群を in-memory SQLite でテスト (`db/db_test.go`):
  - CreateCard → GetCard の往復確認
  - GetOrCreateReview の冪等性確認
  - ListDueCards が due_date フィルタを正しく適用することを確認

---

## Phase 3: SRS (SM-2アルゴリズム)

- [x] `srs/sm2.go` を実装:
  - `CardState` struct: `Interval int`, `EaseFactor float64`, `Repetitions int`
  - `NextState(s CardState, rating int) CardState` — blueprint.md 記載のSM-2ルールを実装
  - `ease_factor` の最小値を `1.3` にクランプ
- [x] `srs/sm2_test.go` を実装 (table-driven):
  - rating=0 でリセットされること
  - rating=2 でリセットされること
  - rating=3 で interval が進むこと
  - rating=5 で interval と ease_factor が正しく増加すること
  - 複数ステップのシーケンス (3回正解後に失敗してリセット)
  - ease_factor が 1.3 を下回らないこと
- [x] `go test ./srs/...` がすべてパスすること

---

## Phase 4: AI層

- [x] `ai/client.go` を実装:
  - `Client` interface: `GenerateHint`, `GenerateCard` メソッド (blueprint.md のシグネチャ通り)
  - `NewClient() (Client, error)` ファクトリ: 環境変数 `AI_BACKEND`, `ANTHROPIC_API_KEY` を読み取る。未設定時は `nil, nil` を返す
- [x] `ai/ollama.go` を実装:
  - `OllamaClient` struct (`baseURL`, `model` フィールド)
  - `net/http` + `encoding/json` で OpenAI互換 `/v1/chat/completions` を呼び出す
  - `GenerateHint`: プロンプト「front: {front}\nback: {back}\nGenerate a short memory hint.」
  - `GenerateCard`: プロンプト「Generate a flashcard about: {topic}. Return JSON {front, back, hint}.」— レスポンスのJSONをパース
  - タイムアウト: 30秒
- [x] `ai/claude.go` を実装:
  - `ClaudeClient` struct
  - `anthropic-sdk-go` を使用、モデル `claude-haiku-4-5` (SDK最新命名)
  - `GenerateHint`, `GenerateCard` の実装 (Ollama と同等のプロンプト)
- [x] `ai/` の単体テスト (`ai/client_test.go`):
  - `AI_BACKEND` が未設定のとき `NewClient` が `nil, nil` を返すことを確認
  - `ANTHROPIC_API_KEY` なしで `AI_BACKEND=claude` のとき `nil, nil` を返すことを確認
  - `Client` インターフェースを満たす `MockClient` を定義し、TUI テスト用に `ai/mock.go` として保存 (build tag `// go:build ignore` は付けない — TUI から import するため)

---

## Phase 5: TUI — ホーム・追加・復習画面

- [x] `tui/app.go` を実装:
  - `screen` 型と定数 (`screenHome`, `screenAdd`, `screenReview`, `screenFetch`)
  - `App` struct (blueprint.md 参照)
  - `New(db *sql.DB, ai ai.Client) *App`
  - `Init()`, `Update()`, `View()` の bubbletea インターフェース実装
  - `Update` で `MsgGotoScreen` メッセージを受け取り `app.screen` を切り替える
  - `Update` で現在の screen のサブモデルに処理を委譲する
  - 各サブモデルから返った `tea.Cmd` をそのまま返す
- [x] `tui/home.go` を実装:
  - `HomeModel` struct
  - `Init()` で DB から total cards 数と due today 数を取得する `tea.Cmd` を返す
  - `View()`: 統計表示 + メニュー (`[r]`, `[a]`, `[f]`, `[i]`, `[q]`)
  - 各キーで対応する `MsgGotoScreen` を発行
- [x] `tui/add.go` を実装:
  - `AddModel` struct、入力ステップ: `stepFront → stepBack → stepHint → stepConfirm`
  - 各ステップで `textarea` or `textinput` (bubbletea 標準コンポーネント) を使用
  - AI client が nil でない場合、`stepHint` で `[g]` キーを押すと `GenerateHint` を呼び出すコマンドを発行、結果をヒントフィールドに設定
  - 確認ステップで `[y]` → `db.CreateCard` を呼び出し HomeScreen に戻る
  - `[esc]` で HomeScreen に戻る (未保存)
- [x] `tui/review.go` を実装:
  - `ReviewModel` struct: `cards []CardWithReview`, `index int`, `revealed bool`
  - `Init()` で `db.ListDueCards` を呼び出すコマンドを発行
  - `View()`: カード表示 (表面のみ → スペースで裏面表示)、評価入力プロンプト
  - 評価キー `0`–`5` → `srs.NextState` 計算 → `db.UpdateReview` → 次のカードへ
  - 全カード終了後: サマリー (「X cards reviewed」) 表示 → `[enter]` で Home
  - 対象カードが0件の場合: 「No cards due today!」表示
- [x] `main.go` を完成:
  - `godotenv` 等は不使用 (環境変数はシェルで設定する想定)
  - `db.Open("anki.db")` → `ai.NewClient()` → `tui.New(db, aiClient)` → `tea.NewProgram(app).Run()`
  - エラーは `log.Fatal`

---

## Phase 6: TUI — 自動コンテンツ取得画面 (FetchModel)

- [x] `tui/fetch.go` を実装:
  - `FetchModel` struct: `input textinput.Model`, `state fetchState` (idle/loading/preview/saved/error)
  - `idle`: トピック入力フォーム、`[enter]` で AI 呼び出し開始
  - `loading`: スピナー表示 (bubbletea spinner コンポーネント)、AI 呼び出しは `tea.Cmd` で非同期実行
  - `preview`: 生成された front/back/hint を表示、`[y]` で保存 / `[n]` でやり直し
  - `saved`: 「Card saved!」表示、1秒後または `[enter]` で Home に戻る
  - `error`: エラーメッセージ表示、`[enter]` でやり直し
  - AI client が nil の場合: 「AI not configured. Set AI_BACKEND and credentials.」を表示して Home に戻るボタンのみ
- [x] `tui/app.go` の Home から `[f]` → FetchScreen 遷移が動作することを確認 (コードレビュー)

---

## Phase 7: CSVインポート

- [x] `importer/csv.go` を実装:
  - `ImportCSV(db *sql.DB, path string) (imported int, err error)`
  - ヘッダ行を必須とする: `front,back,hint` (順不同対応は不要、固定順)
  - `hint` 列が存在しない場合は空文字として扱う (列数が2の場合は hint = "")
  - 各行で `db.CreateCard` を呼び出す
  - 行単位のエラーはスキップしてカウントに含めず、最後にまとめてエラーを返す (partial import を許容)
- [x] `importer/csv_test.go` を実装:
  - 正常系: 3列CSVが全行インポートされること
  - 2列CSV (hint なし) が動作すること
  - 不正な行があっても他の行がインポートされること
- [x] `tui/home.go` の `[i]` キー: ファイルパス入力フォームを表示 → `importer.ImportCSV` 呼び出し → 結果メッセージ表示
  - 実装はシンプルに: `textinput` で1行入力、確定後にインポート実行
  - ホーム画面内のサブ状態 (`homeStateNormal` / `homeStateImport`) で管理 (別 Model は不要)

---

## Phase 8: 統合テスト・動作確認

- [x] `go build ./...` がエラーなく完了すること
- [x] `go test ./...` が全テストパスすること
- [ ] 手動スモークテスト — 以下を順番に実行して確認:
  - [ ] `anki.db` が存在しない状態でアプリ起動 → DB が自動作成されること
  - [ ] Add 画面でカードを1枚追加 → Home に戻り「Total: 1」と表示されること
  - [ ] Review 画面で追加したカードが表示されること → 評価 `5` を入力 → サマリーが表示されること
  - [ ] 翌日分のカードが due になるよう `due_date` を手動で `date('now','-1 day')` に UPDATE し、Reviewが再表示されること
  - [ ] サンプルCSV (`test.csv`: front,back,hint の3行) を作成しインポート → 件数が増えること
  - [ ] `AI_BACKEND=ollama` で Ollama 起動済みの状態で Fetch 画面を開きトピックを入力 → カードが生成・保存されること
  - [ ] `AI_BACKEND=claude` + `ANTHROPIC_API_KEY` 設定で同様に動作すること
  - [ ] 環境変数なし状態でアプリ起動 → AI機能なしで通常動作すること (Fetch 画面で「AI not configured」表示)
- [ ] `README.md` は作成しない (必要になったらユーザーが依頼)
