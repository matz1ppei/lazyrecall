# LazyRecall

TUIで使えるAnkiライクなフラッシュカードアプリ。FSRSアルゴリズムとClaude AI / ローカルLLMを組み合わせた効率的な単語学習を目的とする。

## 技術スタック

| 用途 | ライブラリ |
|---|---|
| 言語 | Go 1.22+ |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) |
| DB | SQLite (`modernc.org/sqlite` — CGO不要) |
| SRS | [go-fsrs](https://github.com/open-spaced-repetition/go-fsrs) |
| AI | Anthropic Claude API / Ollama（切り替え可能） |
| テスト | Go標準 `testing` パッケージ |

## アーキテクチャ

```
lazyrecall/
├── main.go             # エントリポイント
├── ai/                 # AIクライアント（Claude / Ollama / Mock）
├── db/                 # SQLiteアクセス層（cards, reviews, daily_sessions）
├── dict/               # FrequencyWords辞書ローダー
├── importer/           # CSVインポート
├── srs/                # FSRSアルゴリズム（純粋関数）
└── tui/                # Bubble Tea TUI
```

## 開発ルール

- **FSRSアルゴリズム**: `srs/` に集約。副作用なし、純粋関数として実装
- **DB操作**: `db/` 以外からSQLiteに直接アクセスしない
- **AI呼び出し**: `ai/` に集約。APIキーは環境変数 `ANTHROPIC_API_KEY` から取得
- **TUI状態管理**: Bubble Teaの `Model/Update/View` パターンを厳守
- **テスト**: SRS・DB・TUI層はユニットテストを書く。TUIは `Update()` ロジックをテスト対象とする

## 環境変数

| 変数 | 説明 |
|---|---|
| `ANTHROPIC_API_KEY` | Claude API キー |
| `AI_BACKEND` | `claude` または `ollama`（デフォルト: `ollama`） |
| `OLLAMA_HOST` | OllamaサーバーURL（デフォルト: `http://localhost:11434`） |
| `OLLAMA_MODEL` | 使用するOllamaモデル（デフォルト: `qwen2.5:7b`） |

## ビルド・実行

```bash
go run ./main.go
go test ./...
go build -o lazyrecall .
```

## フォーマット・静的解析

```bash
gofmt -w .          # コードフォーマット（変更をファイルに書き戻す）
go vet ./...        # 静的解析（標準ツール）
```

コミット前に必ず両方を実行すること。

## ブランチ・PRワークフロー

```bash
# 1. フィーチャーブランチを作成
git checkout -b feature/xxx

# 2. 実装・コミット
git add <files>
git commit -m "feat: ..."

# 3. リモートに push して PR を作成
git push origin feature/xxx
gh pr create --title "..." --body "..."

# 4. GitHub 上でマージ後、ブランチを削除
git checkout main
git pull origin main
git branch -d feature/xxx
git push origin --delete feature/xxx
```

- ローカルで `git merge` して main に直接取り込まない
- マージは必ず GitHub PR 上で行う
