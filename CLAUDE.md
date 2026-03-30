# poc-anki-claude

TUIで使えるAnkiライクなフラッシュカードアプリ。スペースド・リピティション（間隔反復）を使った効率的な単語学習を目的とする。

## 技術スタック

| 用途 | ライブラリ |
|---|---|
| 言語 | Go 1.22+ |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) |
| DB | SQLite (`modernc.org/sqlite` — CGO不要) |
| AI | Anthropic Claude API (`github.com/anthropics/anthropic-sdk-go`) |
| テスト | Go標準 `testing` パッケージ |

## アーキテクチャ

```
poc-anki-claude/
├── cmd/
│   └── anki/           # エントリポイント (main.go)
├── internal/
│   ├── db/             # SQLiteアクセス層 (cards, reviews テーブル)
│   ├── srs/            # スペースド・リピティション アルゴリズム (SM-2)
│   ├── ai/             # Claude API クライアント (例文生成、単語取得)
│   ├── tui/            # Bubble Tea モデル・ビュー
│   │   ├── home.go     # ホーム画面
│   │   ├── review.go   # 復習セッション
│   │   ├── add.go      # カード登録
│   │   └── styles.go   # Lipgloss スタイル定義
│   └── importer/       # CSVインポート
└── CLAUDE.md
```

## 機能仕様

### コア機能
- **カード登録**: 単語・意味・例文を登録。例文はClaude APIで自動生成
- **復習セッション**: SM-2アルゴリズムによるスペースド・リピティション
- **自動コンテンツ取得**: Claude APIを使って頻出単語リスト（例: スペイン語上位1000語）を自動生成・登録

### 追加機能（あると嬉しい）
- CSVインポート (`internal/importer`)
- マッチングゲーム（Duolingo Match Madnessライク）
- 例文からの画像生成（Stable Diffusion API等）

## 開発ルール

- **SM-2アルゴリズム**: `internal/srs` に集約。副作用なし、純粋関数として実装
- **DB操作**: `internal/db` 以外からSQLiteに直接アクセスしない
- **AI呼び出し**: `internal/ai` に集約。APIキーは環境変数 `ANTHROPIC_API_KEY` から取得
- **TUI状態管理**: Bubble Teaの `Model/Update/View` パターンを厳守
- **テスト**: SRS・DB層はユニットテストを書く。TUIはE2Eが困難なため `Update()` ロジックをテスト対象とする

## 環境変数

| 変数 | 説明 |
|---|---|
| `ANTHROPIC_API_KEY` | Claude API キー（必須） |

## ビルド・実行

```bash
go run ./cmd/anki
go test ./...
go build -o anki ./cmd/anki
```
