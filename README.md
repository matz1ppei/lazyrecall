# LazyRecall

TUIで使えるAnkiライクなフラッシュカードアプリ。スペースド・リピティション（FSRSアルゴリズム）とClaude AI / ローカルLLMを組み合わせた効率的な語彙学習ツール。

## 機能

- **Daily Session** — Review → Brain Dump → Reverse Review → Match Madness → Blank Fill → Retry Reverse の流れで記憶定着を図る
- **4択 Review** — 単語を見て意味を4択で選ぶ
- **Reverse Review** — 意味を見て単語をフリーテキストで入力
- **Retry Reverse** — セッション中に1つでもミスしたカードをセッション末尾でもう1周復習
- **Match Madness** — 単語と意味をペアリングするスピードゲーム
- **Blank Fill** — 例文の空欄に単語を入力
- **カード登録** — 単語・意味・例文を登録。ヒント・例文はAIで自動生成
- **AIバッチ生成** — トピック指定でカードをまとめて自動生成（例: "スペイン語の食べ物"）
- **辞書ベース生成** — FrequencyWordsを使って言語別の頻出語上位N語をインポート
- **CSVインポート** — `front,back,hint` 形式のCSVファイルから一括インポート
- **カード一覧・編集** — 登録済みカードのブラウズ・編集・削除
- **重複除去** — 同一frontを持つカードの一括クリーンアップ

## 技術スタック

| 用途 | ライブラリ |
|---|---|
| 言語 | Go 1.22+ |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lipgloss](https://github.com/charmbracelet/lipgloss) |
| DB | SQLite (`modernc.org/sqlite` — CGO不要) |
| SRS | [FSRS](https://github.com/open-spaced-repetition/go-fsrs) |
| AI | Anthropic Claude API / Ollama（切り替え可能） |

## インストール・実行

### 前提条件

- Go 1.22 以上

### ビルド & 実行

```bash
git clone git@github.com:matz1ppei/lazyrecall.git
cd lazyrecall

# 直接実行
go run ./main.go

# バイナリとしてビルド
go build -o lazyrecall .
./lazyrecall
```

### 環境変数

| 変数 | 説明 | デフォルト |
|---|---|---|
| `ANTHROPIC_API_KEY` | Claude APIキー | — |
| `AI_BACKEND` | `claude` または `ollama` | `ollama` |
| `OLLAMA_HOST` | OllamaサーバーURL | `http://localhost:11434` |
| `OLLAMA_MODEL` | 使用するOllamaモデル | `qwen2.5:7b` |

**Claudeを使う場合:**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export AI_BACKEND=claude
go run ./main.go
```

**Ollamaを使う場合:**

まず [Ollama](https://ollama.com) をインストールします。

```bash
# macOS
brew install ollama

# Linux（公式インストーラ）
curl -fsSL https://ollama.com/install.sh | sh
```

インストール後、モデルを取得して起動します。

```bash
ollama pull qwen2.5:7b
ollama serve   # 別ターミナルで起動（macOSのDesktop App使用時は不要）
go run ./main.go
```

AIバックエンドが未設定でも起動はできますが、AI生成機能は無効になります。

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

**主要な設計方針:**
- FSRSアルゴリズムは `srs/` に純粋関数として実装（副作用なし）
- DB操作は `db/` パッケージに集約
- AI呼び出しは `ai/` パッケージに集約、インターフェースで抽象化
- TUIはBubble TeaのModel/Update/Viewパターンを厳守
- AIなしでも動作するよう graceful degradation を実装

## SRS仕様

### アルゴリズム（FSRS）

スペースド・リピティションに [FSRS](https://github.com/open-spaced-repetition/go-fsrs) を使用。各カードは `stability`（記憶の安定性）と `difficulty`（難易度）で管理される。

正解 → **Good**、不正解 → **Again** にマッピング。Again は stability 低下 + lapses カウントアップ。

### Daily Session

1日1セットが目標。同じ20枚のカードで以下の順番にフェーズを実施する。

1. **Review** — 4択で意味を選ぶ
2. **Brain Dump 1** — 自由回想（記録なし）
3. **Match Madness** — 単語と意味のペアリング
4. **Reverse Review** — 意味を見て単語をタイプ入力
5. **Brain Dump 2** — 自由回想（記録なし）
6. **Blank Fill** — 例文の空欄を入力
7. **Brain Dump 3** — 自由回想 → 終了後に FSRS 採点
8. **Retry Reverse** — 1つでもミスしたカードを Reverse Review で追加1周（全問正解の場合はスキップ）

- 全フェーズ正解（All-correct）→ `UpdateReview(Good)` でインターバルが進む
- 1つでも不正解 → `UpdateReview(Again)` で due_date がリセットされ翌日また出る

### reviewed_at（レビュー記録）の条件

| モード | 記録タイミング |
|---|---|
| Daily Session | セッション最後（Blank完了後）にまとめて記録。途中 `esc` では記録されない |
| Standalone Review | 1問回答するごとに即時記録 |

### カードの状態分類

| 状態 | 条件 |
|---|---|
| New | `reviewed_at IS NULL`（一度もレビューされていない） |
| Learning | `reviewed_at IS NOT NULL` かつ `stability < 21` |
| Mature | `stability >= 21` |

### 各カウンターの定義

| 表示 | 条件 |
|---|---|
| Today: N reviewed | `reviewed_at` が今日の日付のカード数 |
| New Cards | `reviewed_at IS NULL`（一度もレビューされていない） |
| Learning Cards | `reviewed_at IS NOT NULL` かつ `stability < 21` |
| Mature Cards | `stability >= 21` |

## テスト

```bash
go test ./...
```

`srs/`、`db/`、`importer/`、`tui/` にユニットテストがあります。

## キー操作（TUI）

| キー | 操作 |
|---|---|
| `↑` / `↓` / `j` / `k` | 選択移動 |
| `←` / `→` | 列切り替え（Match Madness） |
| `Enter` | 決定 |
| `Ctrl+G` | AI生成（ヒント・例文フィールド） |
| `Ctrl+S` | 保存 |
| `q` / `Esc` | 戻る・終了 |
