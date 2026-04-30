package tui

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ippei/lazyrecall/ai"
	"github.com/ippei/lazyrecall/config"
	"github.com/ippei/lazyrecall/db"
	"github.com/ippei/lazyrecall/debuglog"
)

type sessionPhase int

const (
	sessionPhaseLoading sessionPhase = iota
	sessionPhasePreview              // card survey before session begins
	sessionPhaseReview
	sessionPhaseBrainDump1 // free-recall after Review
	sessionPhaseMatch
	sessionPhaseReverseReview
	sessionPhaseBrainDump2 // free-recall after ReverseReview
	sessionPhaseBlank
	sessionPhaseBrainDump3   // free-recall after Blank
	sessionPhaseRetryReverse // wrong cards から Reverse Review を1周（FSRS採点後）
	sessionPhaseScoring      // FSRS採点中; msgMarkDone を待つ
	sessionPhaseDone
)

const sessionCardLimit = 12

type msgSessionReady struct {
	cards             []db.CardWithReview
	reason            string // non-empty when session cannot start (e.g. DB error, no cards)
	reviewSessionID   int64
	daySessionNo      int
	startErr          string // non-empty when StartReviewSession failed (session continues, but review_sessions row missing)
	phase             sessionPhase
	reviewCorrectIDs  []int64
	reverseCorrectIDs []int64
	matchWrongIDs     []int64
	blankCorrectIDs   []int64
	blankSkipped      bool
	retryCardIDs      []int64
	startedAt         time.Time
	resumed           bool
}

type msgSessionPhaseComplete struct{}

type msgMarkDone struct{}

type msgSessionScored struct {
	err string
}

type msgSessionProgressMarked struct {
	phase string
	err   string
}

// SessionModel orchestrates Preview → Review → BrainDump1 → Match → ReverseReview → Blank → BrainDump2 as a daily session.
type SessionModel struct {
	db                *sql.DB
	ai                ai.Client
	phase             sessionPhase
	quitting          bool // true when esc confirmation dialog is shown
	cards             []db.CardWithReview
	preview           PreviewModel
	review            ReviewModel
	brainDump1        BrainDumpModel
	match             MatchModel
	reverseReview     ReverseInputModel
	brainDump2        BrainDumpModel
	blank             BlankModel
	brainDump3        BrainDumpModel
	reviewDone        bool
	matchDone         bool
	reverseReviewDone bool
	blankDone         bool
	blankSkipped      bool // no cards with translations
	reviewCorrectIDs  []int64
	reverseCorrectIDs []int64
	retryCards        []db.CardWithReview
	retryReview       ReverseInputModel
	retryReviewDone   bool
	reviewSessionID   int64
	daySessionNo      int
	startErr          string // non-empty when StartReviewSession failed
	progressErr       string
	saveErr           string
	startedAt         time.Time
	finishedAt        time.Time
	phaseStartedAt    time.Time
	resumed           bool
}

func NewSessionModel(database *sql.DB, aiClient ai.Client) SessionModel {
	return SessionModel{
		db:    database,
		ai:    aiClient,
		phase: sessionPhaseLoading,
	}
}

func (m SessionModel) Init() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		snapshot, err := config.LoadDailySessionSnapshot()
		today := time.Now().Format("2006-01-02")
		if err == nil && snapshot.Date != "" && snapshot.Date != today {
			_ = config.ClearDailySessionSnapshot()
		}
		if err == nil && snapshot.Date == today && len(snapshot.CardIDs) > 0 {
			cards, cardErr := db.ListCardsWithReviewByIDs(database, snapshot.CardIDs)
			if cardErr == nil {
				daySessionNo, _ := db.GetReviewSessionDayNumber(database, snapshot.ReviewSessionID)
				return msgSessionReady{
					cards:             cards,
					reviewSessionID:   snapshot.ReviewSessionID,
					daySessionNo:      daySessionNo,
					startErr:          snapshot.StartErr,
					phase:             sessionPhaseFromSnapshot(snapshot.Phase),
					reviewCorrectIDs:  snapshot.ReviewCorrectIDs,
					reverseCorrectIDs: snapshot.ReverseCorrectIDs,
					matchWrongIDs:     snapshot.MatchWrongIDs,
					blankCorrectIDs:   snapshot.BlankCorrectIDs,
					blankSkipped:      snapshot.BlankSkipped,
					retryCardIDs:      snapshot.RetryCardIDs,
					startedAt:         snapshot.StartedAt,
					resumed:           true,
				}
			}
			_ = config.ClearDailySessionSnapshot()
		}

		excluded, _ := config.LoadExcludedWords()
		// Fetch extra cards to compensate for exclusions so that after filtering
		// we still reach sessionCardLimit. Due cards are prioritised;
		// overflow is filled with upcoming cards. Cap at 4× to avoid large queries.
		fetchLimit := sessionCardLimit + len(excluded)
		if fetchLimit > sessionCardLimit*4 {
			fetchLimit = sessionCardLimit * 4
		}
		cards, err := db.SelectSessionCards(database, fetchLimit)
		if err != nil {
			return msgSessionReady{reason: fmt.Sprintf("DB error loading cards: %v", err)}
		}
		if len(cards) == 0 {
			return msgSessionReady{reason: "no cards found"}
		}
		var filtered []db.CardWithReview
		for _, c := range cards {
			if !excluded[strings.ToLower(c.Front)] {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) == 0 {
			return msgSessionReady{reason: "all cards are in the exclude list"}
		}
		if len(filtered) > sessionCardLimit {
			filtered = filtered[:sessionCardLimit]
		}
		completedToday, _ := db.CountCompletedDailySessionsToday(database)
		sessionID, err := db.StartReviewSession(database, "daily_session", completedToday+1)
		startErrStr := ""
		if err != nil {
			startErrStr = err.Error()
			debuglog.Errorf("daily_session start failed: %v", err)
		} else {
			mix := db.ClassifyDailySessionCards(filtered)
			if saveErr := db.SaveDailySessionMix(database, sessionID, mix); saveErr != nil {
				debuglog.Errorf("daily_session mix save failed: session_id=%d err=%v", sessionID, saveErr)
			}
			debuglog.Infof("daily_session started: session_id=%d cards=%d day_no=%d", sessionID, len(filtered), completedToday+1)
		}
		return msgSessionReady{cards: filtered, reviewSessionID: sessionID, daySessionNo: completedToday + 1, startErr: startErrStr, phase: sessionPhasePreview}
	}
}

func phaseCompleteCmd() tea.Msg { return msgSessionPhaseComplete{} }

func (m SessionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// esc 確認ダイアログ中は y/n のみ受け付ける
	if m.quitting {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "y":
				return m, func() tea.Msg {
					return MsgGotoScreen{Target: screenHome, Reason: "セッションを中断しました"}
				}
			case "n", "esc":
				m.quitting = false
			}
		}
		return m, nil
	}

	// アクティブなフェーズ中の esc はセッションレベルで先にキャッチ
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
		switch m.phase {
		case sessionPhasePreview, sessionPhaseReview, sessionPhaseBrainDump1,
			sessionPhaseMatch, sessionPhaseReverseReview, sessionPhaseBrainDump2,
			sessionPhaseBlank, sessionPhaseBrainDump3, sessionPhaseRetryReverse:
			m.quitting = true
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case msgSessionReady:
		if len(msg.cards) == 0 {
			reason := msg.reason
			if reason == "" {
				reason = "session ended: no cards"
			}
			debuglog.Infof("daily_session could not start: %s", reason)
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome, Reason: "Session could not start: " + reason} }
		}
		m.cards = msg.cards
		m.reviewSessionID = msg.reviewSessionID
		m.daySessionNo = msg.daySessionNo
		m.startErr = msg.startErr
		m.reviewCorrectIDs = append([]int64(nil), msg.reviewCorrectIDs...)
		m.reverseCorrectIDs = append([]int64(nil), msg.reverseCorrectIDs...)
		m.match.wrongCardIDs = make(map[int64]bool, len(msg.matchWrongIDs))
		for _, id := range msg.matchWrongIDs {
			m.match.wrongCardIDs[id] = true
		}
		m.blank.correctIDs = append([]int64(nil), msg.blankCorrectIDs...)
		m.blankSkipped = msg.blankSkipped
		m.retryCards = filterCardsByID(msg.cards, msg.retryCardIDs)
		m.startedAt = msg.startedAt
		m.resumed = msg.resumed
		phase := msg.phase
		if phase == sessionPhaseLoading {
			phase = sessionPhasePreview
		}
		return m.startPhase(phase)

	case msgSessionPhaseComplete:
		return m.advancePhase()

	case msgMarkDone:
		m.phase = sessionPhaseDone
		m.finishedAt = time.Now()
		_ = config.ClearDailySessionSnapshot()
		debuglog.Infof("daily_session completed: session_id=%d reviewed_cards=%d retry_cards=%d", m.reviewSessionID, len(m.cards), len(m.retryCards))
		return m, nil

	case msgSessionScored:
		if msg.err != "" {
			m.saveErr = msg.err
			m.phase = sessionPhaseDone
			m.finishedAt = time.Now()
			debuglog.Errorf("daily_session scoring failed: session_id=%d err=%s", m.reviewSessionID, msg.err)
			return m, nil
		}
		if len(m.retryCards) > 0 {
			debuglog.Infof("daily_session scoring saved: session_id=%d retry_cards=%d", m.reviewSessionID, len(m.retryCards))
			return m.startPhase(sessionPhaseRetryReverse)
		}
		return m, func() tea.Msg { return msgMarkDone{} }

	case msgSessionProgressMarked:
		if msg.err != "" {
			if m.progressErr == "" {
				m.progressErr = fmt.Sprintf("%s progress save failed: %s", msg.phase, msg.err)
			}
			debuglog.Errorf("daily_session progress save failed: phase=%s err=%s", msg.phase, msg.err)
		}
		return m, nil
	}

	// Forward to active sub-model
	switch m.phase {
	case sessionPhasePreview:
		updated, cmd := m.preview.Update(msg)
		m.preview = updated.(PreviewModel)
		return m, cmd

	case sessionPhaseReview:
		updated, cmd := m.review.Update(msg)
		m.review = updated.(ReviewModel)
		return m, cmd

	case sessionPhaseBrainDump1:
		updated, cmd := m.brainDump1.Update(msg)
		m.brainDump1 = updated.(BrainDumpModel)
		return m, cmd

	case sessionPhaseMatch:
		updated, cmd := m.match.Update(msg)
		m.match = updated.(MatchModel)
		return m, cmd

	case sessionPhaseReverseReview:
		updated, cmd := m.reverseReview.Update(msg)
		m.reverseReview = updated.(ReverseInputModel)
		return m, cmd

	case sessionPhaseBrainDump2:
		updated, cmd := m.brainDump2.Update(msg)
		m.brainDump2 = updated.(BrainDumpModel)
		return m, cmd

	case sessionPhaseBlank:
		updated, cmd := m.blank.Update(msg)
		m.blank = updated.(BlankModel)
		return m, cmd

	case sessionPhaseBrainDump3:
		updated, cmd := m.brainDump3.Update(msg)
		m.brainDump3 = updated.(BrainDumpModel)
		return m, cmd

	case sessionPhaseRetryReverse:
		updated, cmd := m.retryReview.Update(msg)
		m.retryReview = updated.(ReverseInputModel)
		return m, cmd

	case sessionPhaseDone:
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "enter" || key.String() == "esc" || key.String() == " " {
				reason := "Session completed"
				if m.saveErr != "" {
					reason = "Daily Session save failed. See lazyrecall.log."
				}
				return m, func() tea.Msg {
					return MsgGotoScreen{Target: screenHome, Reason: reason}
				}
			}
		}
	}
	return m, nil
}

func (m SessionModel) startPhase(phase sessionPhase) (SessionModel, tea.Cmd) {
	m.phase = phase
	m.phaseStartedAt = time.Now()
	onComplete := tea.Cmd(func() tea.Msg { return msgSessionPhaseComplete{} })
	switch phase {
	case sessionPhasePreview:
		if m.startedAt.IsZero() {
			m.startedAt = time.Now()
		}
		debuglog.Infof("daily_session phase started: preview")
		m.preview = NewPreviewModel(m.cards, onComplete)
		return m, tea.Batch(m.preview.Init(), m.saveSnapshotCmd())
	case sessionPhaseReview:
		debuglog.Infof("daily_session phase started: review")
		m.review = NewReviewModelWithCards(m.db, m.cards, m.reviewSessionID, onComplete)
		initCmd := m.review.Init()
		if m.resumed && m.reviewSessionID != 0 {
			database := m.db
			sid := m.reviewSessionID
			initCmd = tea.Batch(
				func() tea.Msg {
					if err := db.DeleteReviewEventsForSession(database, sid); err != nil {
						debuglog.Errorf("daily_session resume reset failed: session_id=%d err=%v", sid, err)
					}
					return nil
				},
				initCmd,
			)
		}
		return m, tea.Batch(initCmd, m.saveSnapshotCmd())
	case sessionPhaseBrainDump1:
		// BrainDump1 gives the learner a free-recall warm-up after Review.
		// Using extractCards here because BrainDumpModel expects []db.Card (not CardWithReview).
		// Showing first+last letter hints (e.g. "h__a") lowers initial anxiety while still requiring active recall.
		cards1 := extractCards(m.cards)
		debuglog.Infof("daily_session phase started: braindump1")
		m.brainDump1 = NewBrainDumpModel(cards1, "Brain Dump 1", wordShapeHints(cards1), onComplete)
		return m, tea.Batch(m.brainDump1.Init(), m.saveSnapshotCmd())
	case sessionPhaseMatch:
		prevWrongIDs := cloneWrongCardMap(m.match.wrongCardIDs)
		cards := extractCards(m.cards)
		debuglog.Infof("daily_session phase started: match")
		m.match = NewMatchModelWithCards(m.db, cards, onComplete)
		m.match.wrongCardIDs = prevWrongIDs
		return m, tea.Batch(m.match.Init(), m.saveSnapshotCmd())
	case sessionPhaseReverseReview:
		debuglog.Infof("daily_session phase started: reverse_review")
		m.reverseReview = NewReverseInputModelWithCards(m.db, m.cards, onComplete)
		m.reverseReview.correctIDs = append([]int64(nil), m.reverseCorrectIDs...)
		return m, tea.Batch(m.reverseReview.Init(), m.saveSnapshotCmd())
	case sessionPhaseBrainDump2:
		// BrainDump2 runs after ReverseReview. Scores do NOT influence FSRS.
		// Hints show the first letter of all cards.
		cards2 := extractCards(m.cards)
		debuglog.Infof("daily_session phase started: braindump2")
		m.brainDump2 = NewBrainDumpModel(cards2, "Brain Dump 2", firstLetterHints(cards2, nil), onComplete)
		return m, tea.Batch(m.brainDump2.Init(), m.saveSnapshotCmd())
	case sessionPhaseBlank:
		prevBlankCorrectIDs := append([]int64(nil), m.blank.correctIDs...)
		cards := extractCards(m.cards)
		debuglog.Infof("daily_session phase started: blank")
		m.blank = NewBlankModelWithCards(m.db, cards, onComplete)
		m.blank.correctIDs = prevBlankCorrectIDs
		return m, tea.Batch(m.blank.Init(), m.saveSnapshotCmd())
	case sessionPhaseBrainDump3:
		// BrainDump3 runs after Blank as the final recall check before FSRS scoring.
		// Scores here do NOT influence FSRS — only Review/Match/ReverseReview/Blank outcomes do.
		// Hints show the first letter of all cards, matching BD2.
		cards3 := extractCards(m.cards)
		debuglog.Infof("daily_session phase started: braindump3")
		m.brainDump3 = NewBrainDumpModel(cards3, "Brain Dump 3", firstLetterHints(cards3, nil), onComplete)
		return m, tea.Batch(m.brainDump3.Init(), m.saveSnapshotCmd())
	case sessionPhaseRetryReverse:
		// RetryReverse shows wrong cards one more time. FSRS is already scored, so
		// this phase is for reinforcement only — results do not affect scheduling.
		debuglog.Infof("daily_session phase started: retry_reverse cards=%d", len(m.retryCards))
		m.retryReview = NewReverseInputModelWithCards(m.db, m.retryCards, onComplete)
		return m, tea.Batch(m.retryReview.Init(), m.saveSnapshotCmd())
	}
	return m, nil
}

func (m SessionModel) advancePhase() (SessionModel, tea.Cmd) {
	database := m.db
	switch m.phase {
	case sessionPhasePreview:
		debuglog.Infof("daily_session phase completed: preview")
		return m.startPhase(sessionPhaseReview)

	case sessionPhaseReview:
		m.reviewDone = true
		m.reviewCorrectIDs = m.review.correctIDs
		debuglog.Infof("daily_session phase completed: review correct=%d/%d", len(m.reviewCorrectIDs), len(m.cards))
		m.persistPhaseMetric("review", len(m.cards), len(m.reviewCorrectIDs), false)
		// MarkReviewDone is called here (before BrainDump1) so that the daily
		// session progress is recorded regardless of what happens in BrainDump.
		markCmd := func() tea.Msg {
			if err := db.MarkReviewDone(database); err != nil {
				return msgSessionProgressMarked{phase: "review", err: err.Error()}
			}
			return msgSessionProgressMarked{phase: "review"}
		}
		m2, initCmd := m.startPhase(sessionPhaseBrainDump1)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseBrainDump1:
		// BrainDump1 result is intentionally ignored for FSRS — advance straight to Match.
		debuglog.Infof("daily_session phase completed: braindump1 recalled=%d/%d", m.brainDump1.matchCount, m.brainDump1.totalCount)
		m.persistPhaseMetric("braindump1", m.brainDump1.totalCount, m.brainDump1.matchCount, false)
		m2, initCmd := m.startPhase(sessionPhaseMatch)
		return m2, initCmd

	case sessionPhaseMatch:
		m.matchDone = true
		debuglog.Infof("daily_session phase completed: match wrong=%d", len(m.match.wrongCardIDs))
		m.persistPhaseMetric("match", len(m.cards), len(m.cards)-len(m.match.wrongCardIDs), false)
		markCmd := func() tea.Msg {
			if err := db.MarkMatchDone(database); err != nil {
				return msgSessionProgressMarked{phase: "match", err: err.Error()}
			}
			return msgSessionProgressMarked{phase: "match"}
		}
		m2, initCmd := m.startPhase(sessionPhaseReverseReview)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseReverseReview:
		m.reverseReviewDone = true
		m.reverseCorrectIDs = m.reverseReview.correctIDs
		debuglog.Infof("daily_session phase completed: reverse_review correct=%d/%d", len(m.reverseCorrectIDs), len(m.cards))
		m.persistPhaseMetric("reverse_review", len(m.cards), len(m.reverseCorrectIDs), false)
		markCmd := func() tea.Msg {
			if err := db.MarkReverseDone(database); err != nil {
				return msgSessionProgressMarked{phase: "reverse", err: err.Error()}
			}
			return msgSessionProgressMarked{phase: "reverse"}
		}
		m2, initCmd := m.startPhase(sessionPhaseBrainDump2)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseBrainDump2:
		// BrainDump2 result does NOT feed into FSRS — advance to Blank.
		debuglog.Infof("daily_session phase completed: braindump2 recalled=%d/%d", m.brainDump2.matchCount, m.brainDump2.totalCount)
		m.persistPhaseMetric("braindump2", m.brainDump2.totalCount, m.brainDump2.matchCount, false)
		m2, initCmd := m.startPhase(sessionPhaseBlank)
		return m2, initCmd

	case sessionPhaseBlank:
		m.blankDone = true
		if m.blank.state == blankStateEmpty {
			m.blankSkipped = true
		}
		debuglog.Infof("daily_session phase completed: blank correct=%d/%d skipped=%t", len(m.blank.correctIDs), len(m.blank.cards), m.blankSkipped)
		m.persistPhaseMetric("blank", len(m.blank.cards), len(m.blank.correctIDs), m.blankSkipped)
		// MarkBlankDone is called here so daily progress is saved before BrainDump3.
		markCmd := func() tea.Msg {
			if err := db.MarkBlankDone(database); err != nil {
				return msgSessionProgressMarked{phase: "blank", err: err.Error()}
			}
			return msgSessionProgressMarked{phase: "blank"}
		}
		m2, initCmd := m.startPhase(sessionPhaseBrainDump3)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseBrainDump3:
		// BrainDump3 result does NOT feed into FSRS. FSRS scoring uses only
		// Review/Match/ReverseReview/Blank correctness captured above.
		reviewCorrectIDs := m.reviewCorrectIDs
		reverseCorrectIDs := m.reverseCorrectIDs
		matchWrongIDs := m.match.wrongCardIDs
		blankCorrectIDs := m.blank.correctIDs
		cards := m.cards

		// Collect wrong cards for RetryReverse before FSRS scoring runs.
		var retryCards []db.CardWithReview
		for _, cwr := range cards {
			card := cwr.Card
			reviewOK := containsID(reviewCorrectIDs, card.ID)
			matchOK := !matchWrongIDs[card.ID]
			reverseOK := containsID(reverseCorrectIDs, card.ID)
			blankOK := containsID(blankCorrectIDs, card.ID) || card.ExampleTranslation == ""
			if !(reviewOK && matchOK && reverseOK && blankOK) {
				retryCards = append(retryCards, cwr)
			}
		}
		m.retryCards = retryCards

		results := make([]db.SessionResult, 0, len(cards))
		for _, cwr := range cards {
			card := cwr.Card
			reviewOK := containsID(reviewCorrectIDs, card.ID)
			matchOK := !matchWrongIDs[card.ID]
			reverseOK := containsID(reverseCorrectIDs, card.ID)
			blankOK := containsID(blankCorrectIDs, card.ID) || card.ExampleTranslation == ""
			rating := 0
			if reviewOK && matchOK && reverseOK && blankOK {
				rating = 4
			}
			results = append(results, db.SessionResult{CardID: card.ID, Rating: rating})
		}

		debuglog.Infof("daily_session phase completed: braindump3 recalled=%d/%d; saving_results cards=%d retry_cards=%d", m.brainDump3.matchCount, m.brainDump3.totalCount, len(results), len(retryCards))
		m.persistPhaseMetric("braindump3", m.brainDump3.totalCount, m.brainDump3.matchCount, false)

		sid := m.reviewSessionID
		finalPassCount := 0
		for _, result := range results {
			if result.Rating == 4 {
				finalPassCount++
			}
		}
		saveCmd := func() tea.Msg {
			if err := db.ApplySessionResults(database, results, sid, finalPassCount, len(retryCards)); err != nil {
				return msgSessionScored{err: err.Error()}
			}
			return msgSessionScored{}
		}
		m.phase = sessionPhaseScoring
		return m, saveCmd

	case sessionPhaseRetryReverse:
		m.retryReviewDone = true
		debuglog.Infof("daily_session phase completed: retry_reverse correct=%d/%d", len(m.retryReview.correctIDs), len(m.retryCards))
		return m, func() tea.Msg { return msgMarkDone{} }
	}
	return m, nil
}

func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func (m SessionModel) persistPhaseMetric(phase string, itemCount, correctCount int, skipped bool) {
	if m.reviewSessionID == 0 {
		return
	}
	durationSeconds := 0
	if !m.phaseStartedAt.IsZero() {
		durationSeconds = int(math.Round(time.Since(m.phaseStartedAt).Seconds()))
	}
	if err := db.SaveDailySessionPhaseMetric(m.db, m.reviewSessionID, db.DailySessionPhaseMetric{
		Phase:           phase,
		ItemCount:       itemCount,
		CorrectCount:    correctCount,
		DurationSeconds: durationSeconds,
		Skipped:         skipped,
	}); err != nil {
		debuglog.Errorf("daily_session phase metric save failed: session_id=%d phase=%s err=%v", m.reviewSessionID, phase, err)
	}
}

func extractCards(cwrs []db.CardWithReview) []db.Card {
	cards := make([]db.Card, len(cwrs))
	for i, cwr := range cwrs {
		cards[i] = cwr.Card
	}
	return cards
}

func filterCardsByID(cards []db.CardWithReview, ids []int64) []db.CardWithReview {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[int64]db.CardWithReview, len(cards))
	for _, card := range cards {
		byID[card.Card.ID] = card
	}
	filtered := make([]db.CardWithReview, 0, len(ids))
	for _, id := range ids {
		if card, ok := byID[id]; ok {
			filtered = append(filtered, card)
		}
	}
	return filtered
}

func cloneWrongCardMap(src map[int64]bool) map[int64]bool {
	if len(src) == 0 {
		return make(map[int64]bool)
	}
	dst := make(map[int64]bool, len(src))
	for id, wrong := range src {
		dst[id] = wrong
	}
	return dst
}

func (m SessionModel) snapshotPhaseName() string {
	switch m.phase {
	case sessionPhasePreview:
		return "preview"
	case sessionPhaseReview:
		return "review"
	case sessionPhaseBrainDump1:
		return "braindump1"
	case sessionPhaseMatch:
		return "match"
	case sessionPhaseReverseReview:
		return "reverse_review"
	case sessionPhaseBrainDump2:
		return "braindump2"
	case sessionPhaseBlank:
		return "blank"
	case sessionPhaseBrainDump3:
		return "braindump3"
	case sessionPhaseRetryReverse:
		return "retry_reverse"
	default:
		return ""
	}
}

func sessionPhaseFromSnapshot(name string) sessionPhase {
	switch name {
	case "preview":
		return sessionPhasePreview
	case "review":
		return sessionPhaseReview
	case "braindump1":
		return sessionPhaseBrainDump1
	case "match":
		return sessionPhaseMatch
	case "reverse_review":
		return sessionPhaseReverseReview
	case "braindump2":
		return sessionPhaseBrainDump2
	case "blank":
		return sessionPhaseBlank
	case "braindump3":
		return sessionPhaseBrainDump3
	case "retry_reverse":
		return sessionPhaseRetryReverse
	default:
		return sessionPhasePreview
	}
}

func (m SessionModel) saveSnapshotCmd() tea.Cmd {
	cardIDs := make([]int64, 0, len(m.cards))
	for _, card := range m.cards {
		cardIDs = append(cardIDs, card.Card.ID)
	}
	matchWrongIDs := make([]int64, 0, len(m.match.wrongCardIDs))
	for id, wrong := range m.match.wrongCardIDs {
		if wrong {
			matchWrongIDs = append(matchWrongIDs, id)
		}
	}
	retryCardIDs := make([]int64, 0, len(m.retryCards))
	for _, card := range m.retryCards {
		retryCardIDs = append(retryCardIDs, card.Card.ID)
	}
	snapshot := config.DailySessionSnapshot{
		Date:              time.Now().Format("2006-01-02"),
		CardIDs:           cardIDs,
		ReviewSessionID:   m.reviewSessionID,
		Phase:             m.snapshotPhaseName(),
		ReviewCorrectIDs:  append([]int64(nil), m.reviewCorrectIDs...),
		ReverseCorrectIDs: append([]int64(nil), m.reverseCorrectIDs...),
		MatchWrongIDs:     matchWrongIDs,
		BlankCorrectIDs:   append([]int64(nil), m.blank.correctIDs...),
		BlankSkipped:      m.blankSkipped,
		RetryCardIDs:      retryCardIDs,
		StartedAt:         m.startedAt,
		StartErr:          m.startErr,
	}
	return func() tea.Msg {
		if snapshot.Phase == "" {
			return nil
		}
		if err := config.SaveDailySessionSnapshot(snapshot); err != nil {
			debuglog.Errorf("daily_session snapshot save failed: phase=%s err=%v", snapshot.Phase, err)
		}
		return nil
	}
}

func (m SessionModel) View() string {
	if m.quitting {
		var b strings.Builder
		b.WriteString(titleStyle.Render("Daily Session"))
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render("セッションを中断しますか？"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("%s  %s", keyStyle.Render("[y] 中断"), keyStyle.Render("[n] 続ける")))
		return b.String()
	}

	switch m.phase {
	case sessionPhaseLoading:
		return titleStyle.Render("Daily Session") + "\n\n" + subtitleStyle.Render("Loading cards...")
	case sessionPhasePreview:
		return m.preview.View()
	case sessionPhaseReview:
		return m.review.View()
	case sessionPhaseBrainDump1:
		return m.brainDump1.View()
	case sessionPhaseMatch:
		return m.match.View()
	case sessionPhaseReverseReview:
		return m.reverseReview.View()
	case sessionPhaseBrainDump2:
		return m.brainDump2.View()
	case sessionPhaseBlank:
		return m.blank.View()
	case sessionPhaseBrainDump3:
		return m.brainDump3.View()
	case sessionPhaseRetryReverse:
		return m.retryReview.View()
	case sessionPhaseScoring:
		return titleStyle.Render("Daily Session") + "\n\n" + subtitleStyle.Render("Saving results...")
	case sessionPhaseDone:
		return m.viewDone()
	}
	return ""
}

func (m SessionModel) viewDone() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Daily Session Complete!"))
	b.WriteString("\n\n")

	type phaseStatus struct {
		label string
		done  bool
		note  string
	}
	phases := []phaseStatus{
		{"Review", m.reviewDone, ""},
		{"Match Madness", m.matchDone, ""},
		{"Reverse Review", m.reverseReviewDone, ""},
		{"Blank fill", m.blankDone, func() string {
			if m.blankSkipped {
				return " (no translations)"
			}
			return ""
		}()},
	}
	if len(m.retryCards) > 0 {
		phases = append(phases, phaseStatus{"Retry Reverse", m.retryReviewDone, fmt.Sprintf(" (%d cards)", len(m.retryCards))})
	}

	for _, p := range phases {
		if p.done {
			b.WriteString(successStyle.Render("✓ " + p.label + p.note))
		} else {
			b.WriteString(subtitleStyle.Render("✗ " + p.label))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	allDone := m.reviewDone && m.matchDone && m.reverseReviewDone && m.blankDone
	anyDone := m.reviewDone || m.matchDone || m.reverseReviewDone || m.blankDone

	switch {
	case len(m.cards) == 0:
		b.WriteString(subtitleStyle.Render("No cards yet. Add some cards first!"))
	case m.saveErr != "":
		b.WriteString(errorStyle.Render("Session finished, but final results were not saved."))
	case m.daySessionNo >= dailySessionIdealGoal:
		b.WriteString(idealStyle.Render(fmt.Sprintf("Ideal reached! Daily Session %d / %d complete.", m.daySessionNo, dailySessionIdealGoal)))
	case allDone:
		b.WriteString(successStyle.Render("Minimum reached! Today's Daily Session is done."))
	case anyDone:
		done := 0
		if m.reviewDone {
			done++
		}
		if m.matchDone {
			done++
		}
		if m.reverseReviewDone {
			done++
		}
		if m.blankDone {
			done++
		}
		b.WriteString(labelStyle.Render(fmt.Sprintf("Streak continues! (%d / 4 phases complete)", done)))
	default:
		b.WriteString(labelStyle.Render("Session complete."))
	}
	b.WriteString("\n\n")
	if !m.startedAt.IsZero() && !m.finishedAt.IsZero() {
		elapsed := m.finishedAt.Sub(m.startedAt)
		mins := int(math.Floor(elapsed.Minutes()))
		secs := int(elapsed.Seconds()) % 60
		b.WriteString(labelStyle.Render(fmt.Sprintf("Time: %dm %02ds", mins, secs)))
		b.WriteString("\n\n")
	}
	if m.startErr != "" {
		b.WriteString(helpStyle.Render("⚠ session log error: " + m.startErr))
		b.WriteString("\n")
	}
	if m.progressErr != "" {
		b.WriteString(errorStyle.Render("Progress warning: " + m.progressErr))
		b.WriteString("\n")
	}
	if m.saveErr != "" {
		b.WriteString(errorStyle.Render("Save error: " + m.saveErr))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("See lazyrecall.log for details."))
		b.WriteString("\n")
	}
	b.WriteString(helpStyle.Render("[enter] back to home"))
	return b.String()
}
