# Architectural Decision Records

## ADR-001: Tech Stack Selection
- **日付**: 2026-03-30
- **状態**: 採用
- **背景**: PoC として Anki クローンを構築するにあたり、実行バイナリが単一ファイルになること、依存が少ないこと、TUI フレームワークの品質が高いことが求められた。
- **決定**: Go 1.22+ / bubbletea (TUI) / lipgloss (スタイル) / modernc.org/sqlite (CGO-free) / net/http (stdlib)
- **理由**: Go は単一バイナリ配布が容易で CGO-free の sqlite ドライバにより外部ライブラリ不要。bubbletea は Elm アーキテクチャに基づき状態管理が明確。lipgloss との組み合わせでターミナル UI の実装コストが低い。
- **影響**: クロスコンパイルが容易になる。CGO 不要のため CI 環境構築が単純。TUI 実装はユニットテスト対象外とし手動スモークテストで検証する。

## ADR-002: AI バックエンドの切り替え設計
- **日付**: 2026-03-30
- **状態**: 採用
- **背景**: PoC 段階ではローカル LLM (Ollama) で開発・テストし、必要に応じて Anthropic Claude API へ切り替えられる柔軟性が必要だった。API キーなしでも AI 機能を無効化して動作できることも要件。
- **決定**: `ai.Client` インターフェースを定義し、`AI_BACKEND` 環境変数で `ollama` / `claude` を実行時に選択する。キー未設定時は `nil` を返し AI 機能を無効化する（エラーにしない）。
- **理由**: インターフェースによる抽象化でテスト用モック (`ai/mock.go`) が差し込み可能になる。環境変数による切り替えはデプロイ設定の変更のみで済み、コード変更が不要。`nil` チェックによる graceful degradation でオフライン環境でも基本機能が動作する。
- **影響**: 新しいバックエンド（OpenAI 等）の追加が `ai.Client` を実装するだけで可能。TUI 側は `ai.Client` が `nil` かどうかを確認してメニュー表示を制御する必要がある。
