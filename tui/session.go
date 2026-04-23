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
	"github.com/ippei/lazyrecall/srs"
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
	sessionPhaseDone
)

const sessionCardLimit = 12

type msgSessionReady struct {
	cards           []db.CardWithReview
	reason          string // non-empty when session cannot start (e.g. DB error, no cards)
	reviewSessionID int64
}

type msgSessionPhaseComplete struct{}

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
	startedAt         time.Time
	finishedAt        time.Time
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
		dayNo, _ := db.CountTodayReviewSessions(database)
		sessionID, _ := db.StartReviewSession(database, "daily_session", dayNo+1)
		return msgSessionReady{cards: filtered, reviewSessionID: sessionID}
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
			return m, func() tea.Msg { return MsgGotoScreen{Target: screenHome, Reason: "Session could not start: " + reason} }
		}
		m.cards = msg.cards
		m.reviewSessionID = msg.reviewSessionID
		return m.startPhase(sessionPhasePreview)

	case msgSessionPhaseComplete:
		return m.advancePhase()
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
				return m, func() tea.Msg {
					return MsgGotoScreen{Target: screenHome, Reason: "Session completed"}
				}
			}
		}
	}
	return m, nil
}

func (m SessionModel) startPhase(phase sessionPhase) (SessionModel, tea.Cmd) {
	m.phase = phase
	onComplete := tea.Cmd(func() tea.Msg { return msgSessionPhaseComplete{} })
	switch phase {
	case sessionPhasePreview:
		m.startedAt = time.Now()
		m.preview = NewPreviewModel(m.cards, onComplete)
		return m, m.preview.Init()
	case sessionPhaseReview:
		m.review = NewReviewModelWithCards(m.db, m.cards, m.reviewSessionID, onComplete)
		return m, m.review.Init()
	case sessionPhaseBrainDump1:
		// BrainDump1 gives the learner a free-recall warm-up after Review.
		// Using extractCards here because BrainDumpModel expects []db.Card (not CardWithReview).
		m.brainDump1 = NewBrainDumpModel(extractCards(m.cards), "Brain Dump 1", onComplete)
		return m, m.brainDump1.Init()
	case sessionPhaseMatch:
		cards := extractCards(m.cards)
		m.match = NewMatchModelWithCards(m.db, cards, onComplete)
		return m, m.match.Init()
	case sessionPhaseReverseReview:
		m.reverseReview = NewReverseInputModelWithCards(m.db, m.cards, onComplete)
		return m, m.reverseReview.Init()
	case sessionPhaseBrainDump2:
		// BrainDump2 runs after ReverseReview. Scores do NOT influence FSRS.
		m.brainDump2 = NewBrainDumpModel(extractCards(m.cards), "Brain Dump 2", onComplete)
		return m, m.brainDump2.Init()
	case sessionPhaseBlank:
		cards := extractCards(m.cards)
		m.blank = NewBlankModelWithCards(m.db, cards, onComplete)
		return m, m.blank.Init()
	case sessionPhaseBrainDump3:
		// BrainDump3 runs after Blank as the final recall check before FSRS scoring.
		// Scores here do NOT influence FSRS — only Review/Match/ReverseReview/Blank outcomes do.
		m.brainDump3 = NewBrainDumpModel(extractCards(m.cards), "Brain Dump 3", onComplete)
		return m, m.brainDump3.Init()
	case sessionPhaseRetryReverse:
		// RetryReverse shows wrong cards one more time. FSRS is already scored, so
		// this phase is for reinforcement only — results do not affect scheduling.
		m.retryReview = NewReverseInputModelWithCards(m.db, m.retryCards, onComplete)
		return m, m.retryReview.Init()
	}
	return m, nil
}

func (m SessionModel) advancePhase() (SessionModel, tea.Cmd) {
	database := m.db
	switch m.phase {
	case sessionPhasePreview:
		return m.startPhase(sessionPhaseReview)

	case sessionPhaseReview:
		m.reviewDone = true
		m.reviewCorrectIDs = m.review.correctIDs
		// MarkReviewDone is called here (before BrainDump1) so that the daily
		// session progress is recorded regardless of what happens in BrainDump.
		markCmd := func() tea.Msg { db.MarkReviewDone(database); return nil }
		m2, initCmd := m.startPhase(sessionPhaseBrainDump1)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseBrainDump1:
		// BrainDump1 result is intentionally ignored for FSRS — advance straight to Match.
		return m.startPhase(sessionPhaseMatch)

	case sessionPhaseMatch:
		m.matchDone = true
		markCmd := func() tea.Msg { db.MarkMatchDone(database); return nil }
		m2, initCmd := m.startPhase(sessionPhaseReverseReview)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseReverseReview:
		m.reverseReviewDone = true
		m.reverseCorrectIDs = m.reverseReview.correctIDs
		markCmd := func() tea.Msg { db.MarkReverseDone(database); return nil }
		m2, initCmd := m.startPhase(sessionPhaseBrainDump2)
		return m2, tea.Batch(markCmd, initCmd)

	case sessionPhaseBrainDump2:
		// BrainDump2 result does NOT feed into FSRS — advance to Blank.
		return m.startPhase(sessionPhaseBlank)

	case sessionPhaseBlank:
		m.blankDone = true
		if m.blank.state == blankStateEmpty {
			m.blankSkipped = true
		}
		// MarkBlankDone is called here so daily progress is saved before BrainDump3.
		markCmd := func() tea.Msg { db.MarkBlankDone(database); return nil }
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

		markCmd := func() tea.Msg {
			for _, cwr := range cards {
				card := cwr.Card
				reviewOK := containsID(reviewCorrectIDs, card.ID)
				matchOK := !matchWrongIDs[card.ID]
				reverseOK := containsID(reverseCorrectIDs, card.ID)
				blankOK := containsID(blankCorrectIDs, card.ID) || card.ExampleTranslation == ""
				if reviewOK && matchOK && reverseOK && blankOK {
					markGood(database, card.ID)
				} else {
					markAgain(database, card.ID)
				}
			}
			return nil
		}

		endCmd := m.endReviewSessionCmd()
		if len(retryCards) > 0 {
			// Run FSRS scoring in background, then start RetryReverse.
			m2, initCmd := m.startPhase(sessionPhaseRetryReverse)
			return m2, tea.Batch(markCmd, initCmd, endCmd)
		}
		// No wrong cards — go straight to Done.
		m.phase = sessionPhaseDone
		m.finishedAt = time.Now()
		return m, tea.Batch(markCmd, endCmd)

	case sessionPhaseRetryReverse:
		m.retryReviewDone = true
		m.phase = sessionPhaseDone
		m.finishedAt = time.Now()
		return m, nil
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

// markAgain applies FSRS Again for a card that was not triple-correct.
// This lowers stability, increments lapses, and schedules a short re-learning interval.
func (m SessionModel) endReviewSessionCmd() tea.Cmd {
	if m.reviewSessionID == 0 {
		return nil
	}
	database := m.db
	sid := m.reviewSessionID
	return func() tea.Msg {
		_ = db.EndReviewSession(database, sid)
		return nil
	}
}

func markAgain(database *sql.DB, cardID int64) {
	r, err := db.GetOrCreateReview(database, cardID)
	if err != nil {
		return
	}
	result := srs.Schedule(db.ReviewToSRS(r), srs.RatingAgain, time.Now())
	db.ApplySRSResult(&r, result)
	rating := 0
	r.LastRating = &rating
	db.UpdateReview(database, r)
}

// markGood applies FSRS Good for a triple-correct card, advancing its interval.
func markGood(database *sql.DB, cardID int64) {
	r, err := db.GetOrCreateReview(database, cardID)
	if err != nil {
		return
	}
	result := srs.Schedule(db.ReviewToSRS(r), srs.RatingGood, time.Now())
	db.ApplySRSResult(&r, result)
	rating := 4
	r.LastRating = &rating
	db.UpdateReview(database, r)
}

func extractCards(cwrs []db.CardWithReview) []db.Card {
	cards := make([]db.Card, len(cwrs))
	for i, cwr := range cwrs {
		cards[i] = cwr.Card
	}
	return cards
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
	case allDone:
		b.WriteString(successStyle.Render(fmt.Sprintf("Goal achieved! All %d cards covered.", len(m.cards))))
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
	}
	b.WriteString("\n\n")
	if !m.startedAt.IsZero() && !m.finishedAt.IsZero() {
		elapsed := m.finishedAt.Sub(m.startedAt)
		mins := int(math.Floor(elapsed.Minutes()))
		secs := int(elapsed.Seconds()) % 60
		b.WriteString(labelStyle.Render(fmt.Sprintf("Time: %dm %02ds", mins, secs)))
		b.WriteString("\n\n")
	}
	b.WriteString(helpStyle.Render("[enter] back to home"))
	return b.String()
}
