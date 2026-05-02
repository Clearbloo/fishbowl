package main

import (
	"bytes"
	cryptorand "crypto/rand"
	"fmt"
	"html/template"
	"log"
	mathrand "math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Player struct {
	ID    string
	Name  string
	Score int
}

type SSEEvent struct {
	Name string
	Data string
}

type GameState struct {
	mu               sync.Mutex
	Words            []string
	AllPhaseWords    []string
	PhaseWords       []string
	Round            []string
	CurrentWord      string
	Started          bool
	RoundStarted     bool
	StartTime        *time.Time
	RoundLength      time.Duration
	Players          []Player
	CurrentPlayerIdx int
	PhaseNumber      int
	RoundScored      int
	Clients          map[string]chan SSEEvent
	timerStop        chan struct{}
	tmpl             *template.Template
}

func newGameState(tmpl *template.Template) *GameState {
	return &GameState{
		RoundLength: 30 * time.Second,
		Clients:     make(map[string]chan SSEEvent),
		tmpl:        tmpl,
		PhaseNumber: 1,
	}
}

func (s *GameState) reset() {
	s.stopTimer()
	s.Words = nil
	s.AllPhaseWords = nil
	s.PhaseWords = nil
	s.Round = nil
	s.CurrentWord = ""
	s.Started = false
	s.RoundStarted = false
	s.StartTime = nil
	s.Players = nil
	s.CurrentPlayerIdx = 0
	s.PhaseNumber = 1
	s.RoundScored = 0
}

func (s *GameState) renderToString(name string, data interface{}) string {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Sprintf("<!-- template error: %v -->", err)
	}
	return buf.String()
}

func (s *GameState) broadcastAll(evt SSEEvent) {
	for _, ch := range s.Clients {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (s *GameState) broadcastExcept(evt SSEEvent, excludeID string) {
	for id, ch := range s.Clients {
		if id == excludeID {
			continue
		}
		select {
		case ch <- evt:
		default:
		}
	}
}

func (s *GameState) broadcastToPlayer(sessionID string, evt SSEEvent) {
	if ch, ok := s.Clients[sessionID]; ok {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (s *GameState) activePlayerID() string {
	if len(s.Players) == 0 {
		return ""
	}
	return s.Players[s.CurrentPlayerIdx].ID
}

func (s *GameState) activePlayerName() string {
	if len(s.Players) == 0 {
		return ""
	}
	return s.Players[s.CurrentPlayerIdx].Name
}

func (s *GameState) drawWord() string {
	if len(s.Round) == 0 {
		return ""
	}
	i := mathrand.Intn(len(s.Round))
	word := s.Round[i]
	s.Round = append(s.Round[:i], s.Round[i+1:]...)
	s.CurrentWord = word
	return word
}

func (s *GameState) stopTimer() {
	if s.timerStop != nil {
		select {
		case <-s.timerStop:
		default:
			close(s.timerStop)
		}
		s.timerStop = nil
	}
}

// endRound cleans up round state and returns the summary HTML.
// Must be called with s.mu held.
func (s *GameState) endRound() string {
	s.RoundStarted = false
	s.StartTime = nil
	s.stopTimer()
	s.Round = nil
	s.CurrentWord = ""

	players := make([]Player, len(s.Players))
	copy(players, s.Players)
	return s.renderToString("round-summary.html", gin.H{
		"players":       players,
		"scored":        s.RoundScored,
		"activeName":    s.activePlayerName(),
		"phaseComplete": len(s.PhaseWords) == 0,
	})
}

func (s *GameState) startTimer() {
	s.stopTimer()
	stop := make(chan struct{})
	s.timerStop = stop
	startTime := *s.StartTime
	roundLength := s.RoundLength

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case t := <-ticker.C:
				remaining := roundLength - t.Sub(startTime)

				s.mu.Lock()
				if !s.RoundStarted {
					s.mu.Unlock()
					return
				}

				if remaining <= 0 {
					s.RoundStarted = false
					s.StartTime = nil
					s.Round = nil
					s.CurrentWord = ""

					players := make([]Player, len(s.Players))
					copy(players, s.Players)
					summaryHTML := s.renderToString("round-summary.html", gin.H{
						"players":       players,
						"scored":        s.RoundScored,
						"activeName":    s.activePlayerName(),
						"phaseComplete": len(s.PhaseWords) == 0,
					})
					clockHTML := s.renderToString("clock.tmpl", gin.H{"stopped": true})
					s.broadcastAll(SSEEvent{Name: "clock", Data: clockHTML})
					s.broadcastAll(SSEEvent{Name: "game", Data: summaryHTML})
					s.mu.Unlock()
					return
				}

				wordsLeft := len(s.PhaseWords)
				clockHTML := s.renderToString("clock.tmpl", gin.H{
					"time":      int(remaining.Seconds()) + 1,
					"wordsleft": wordsLeft,
				})
				s.broadcastAll(SSEEvent{Name: "clock", Data: clockHTML})
				s.mu.Unlock()
			}
		}
	}()
}

func generateSessionID() string {
	b := make([]byte, 16)
	cryptorand.Read(b)
	return fmt.Sprintf("%x", b)
}

func phaseName(phase int) string {
	switch phase {
	case 1:
		return "Phase 1: Describe"
	case 2:
		return "Phase 2: Act It Out"
	case 3:
		return "Phase 3: One Word"
	}
	return ""
}

func phaseRules(phase int) string {
	switch phase {
	case 1:
		return "Describe the word using any words except the word itself"
	case 2:
		return "Act it out — no talking, no sounds, no mouthing"
	case 3:
		return "Say exactly one word as your clue"
	}
	return ""
}

func findWinner(players []Player) string {
	maxScore := -1
	winner := ""
	for _, p := range players {
		if p.Score > maxScore {
			maxScore = p.Score
			winner = p.Name
		}
	}
	return winner
}

func main() {
	tmpl := template.Must(template.ParseGlob("templates/*"))
	state := newGameState(tmpl)

	r := gin.New()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")
	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/events"},
	}))

	r.GET("/", func(c *gin.Context) {
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessionID == "" {
			sessionID = generateSessionID()
			c.SetCookie("session_id", sessionID, 86400*7, "/", "", false, false)
		}
		c.File("templates/index.html")
	})

	r.GET("/events", func(c *gin.Context) {
		sessionID, _ := c.Cookie("session_id")
		ch := make(chan SSEEvent, 10)

		state.mu.Lock()
		state.Clients[sessionID] = ch
		state.mu.Unlock()

		defer func() {
			state.mu.Lock()
			if state.Clients[sessionID] == ch {
				delete(state.Clients, sessionID)
			}
			state.mu.Unlock()
		}()

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		for {
			select {
			case evt := <-ch:
				fmt.Fprintf(c.Writer, "event: %s\n", evt.Name)
				for _, line := range strings.Split(evt.Data, "\n") {
					fmt.Fprintf(c.Writer, "data: %s\n", line)
				}
				fmt.Fprintf(c.Writer, "\n")
				c.Writer.Flush()
			case <-c.Request.Context().Done():
				return
			}
		}
	})

	r.POST("/player", func(c *gin.Context) {
		sessionID, _ := c.Cookie("session_id")
		name := strings.TrimSpace(c.PostForm("name"))
		if name == "" {
			c.String(http.StatusBadRequest, "Name required")
			return
		}

		state.mu.Lock()
		defer state.mu.Unlock()

		if state.Started {
			c.String(http.StatusLocked, "Game already started")
			return
		}

		for i, p := range state.Players {
			if p.ID == sessionID {
				state.Players[i].Name = name
				playersHTML := state.renderToString("player-list.html", gin.H{"players": state.Players})
				state.broadcastExcept(SSEEvent{Name: "players", Data: playersHTML}, sessionID)
				c.HTML(http.StatusOK, "player-list.html", gin.H{"players": state.Players})
				return
			}
		}
		state.Players = append(state.Players, Player{ID: sessionID, Name: name})
		playersHTML := state.renderToString("player-list.html", gin.H{"players": state.Players})
		state.broadcastExcept(SSEEvent{Name: "players", Data: playersHTML}, sessionID)
		c.HTML(http.StatusOK, "player-list.html", gin.H{"players": state.Players})
	})

	r.GET("/players", func(c *gin.Context) {
		state.mu.Lock()
		players := make([]Player, len(state.Players))
		copy(players, state.Players)
		state.mu.Unlock()
		c.HTML(http.StatusOK, "player-list.html", gin.H{"players": players})
	})

	r.POST("/word", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		if state.Started {
			c.String(http.StatusLocked, "Game already started")
			return
		}
		word := strings.TrimSpace(c.PostForm("word"))
		if word == "" {
			c.String(http.StatusBadRequest, "Word required")
			return
		}
		state.Words = append(state.Words, word)
		log.Printf("Words in bowl: %d", len(state.Words))
		c.String(http.StatusOK, "Added: "+word)
	})

	r.GET("/start-game", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		if len(state.Players) < 2 {
			c.String(http.StatusBadRequest, "Need at least 2 players")
			return
		}
		if len(state.Words) == 0 {
			c.String(http.StatusBadRequest, "Need at least 1 word")
			return
		}

		state.Started = true
		state.PhaseNumber = 1
		state.AllPhaseWords = make([]string, len(state.Words))
		copy(state.AllPhaseWords, state.Words)
		state.PhaseWords = make([]string, len(state.Words))
		copy(state.PhaseWords, state.Words)

		sessionID, _ := c.Cookie("session_id")
		data := gin.H{
			"phaseName":  phaseName(1),
			"phaseRules": phaseRules(1),
			"activeName": state.activePlayerName(),
		}
		html := state.renderToString("game-page.html", data)
		state.broadcastExcept(SSEEvent{Name: "game", Data: html}, sessionID)
		c.HTML(http.StatusOK, "game-page.html", data)
	})

	r.GET("/start-round", func(c *gin.Context) {
		state.mu.Lock()

		if !state.Started || state.RoundStarted {
			state.mu.Unlock()
			c.String(http.StatusConflict, "Cannot start round now")
			return
		}
		if len(state.PhaseWords) == 0 {
			state.mu.Unlock()
			c.String(http.StatusConflict, "No words left")
			return
		}

		now := time.Now()
		state.StartTime = &now
		state.RoundStarted = true
		state.RoundScored = 0
		state.Round = make([]string, len(state.PhaseWords))
		copy(state.Round, state.PhaseWords)

		word := state.drawWord()
		sessionID, _ := c.Cookie("session_id")
		activeID := state.activePlayerID()
		activeName := state.activePlayerName()
		wordsLeft := len(state.PhaseWords)

		state.startTimer()

		// Push initial clock to all
		clockHTML := state.renderToString("clock.tmpl", gin.H{
			"time":      30,
			"wordsleft": wordsLeft,
		})
		state.broadcastAll(SSEEvent{Name: "clock", Data: clockHTML})

		// Push guesser view to non-active clients
		guesserHTML := state.renderToString("guesser-view.html", gin.H{"activeName": activeName})
		for id, ch := range state.Clients {
			if id != sessionID {
				select {
				case ch <- SSEEvent{Name: "game", Data: guesserHTML}:
				default:
				}
			}
		}

		state.mu.Unlock()

		if sessionID == activeID {
			c.HTML(http.StatusOK, "active-view.html", gin.H{"word": word})
		} else {
			c.HTML(http.StatusOK, "guesser-view.html", gin.H{"activeName": activeName})
		}
	})

	r.POST("/score", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		sessionID, _ := c.Cookie("session_id")
		if !state.RoundStarted {
			c.String(http.StatusConflict, "Round not active")
			return
		}
		if sessionID != state.activePlayerID() {
			c.String(http.StatusForbidden, "Not your turn")
			return
		}

		// Remove guessed word from PhaseWords
		for i, w := range state.PhaseWords {
			if w == state.CurrentWord {
				state.PhaseWords = append(state.PhaseWords[:i], state.PhaseWords[i+1:]...)
				break
			}
		}

		state.Players[state.CurrentPlayerIdx].Score++
		state.RoundScored++

		// Check if all phase words are guessed
		if len(state.PhaseWords) == 0 {
			summaryHTML := state.endRound()
			clockHTML := state.renderToString("clock.tmpl", gin.H{"stopped": true})
			state.broadcastAll(SSEEvent{Name: "clock", Data: clockHTML})
			state.broadcastExcept(SSEEvent{Name: "game", Data: summaryHTML}, sessionID)
			c.Header("HX-Retarget", "#game")
			c.Header("HX-Reswap", "innerHTML")
			c.HTML(http.StatusOK, "round-summary.html", gin.H{
				"players":       state.Players,
				"scored":        state.RoundScored,
				"activeName":    state.activePlayerName(),
				"phaseComplete": true,
			})
			return
		}

		// Draw next word and return active view
		nextWord := state.drawWord()
		c.HTML(http.StatusOK, "active-view.html", gin.H{"word": nextWord})
	})

	r.GET("/skip", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		sessionID, _ := c.Cookie("session_id")
		if !state.RoundStarted {
			c.String(http.StatusConflict, "Round not active")
			return
		}
		if sessionID != state.activePlayerID() {
			c.String(http.StatusForbidden, "Not your turn")
			return
		}
		if len(state.Round) == 0 {
			// Only one word left, can't skip
			c.String(http.StatusOK, state.CurrentWord)
			return
		}

		state.Round = append(state.Round, state.CurrentWord)
		nextWord := state.drawWord()
		c.String(http.StatusOK, nextWord)
	})

	r.GET("/next-turn", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		state.CurrentPlayerIdx = (state.CurrentPlayerIdx + 1) % len(state.Players)
		state.RoundScored = 0
		sessionID, _ := c.Cookie("session_id")

		if len(state.PhaseWords) == 0 {
			if state.PhaseNumber >= 3 {
				players := make([]Player, len(state.Players))
				copy(players, state.Players)
				winner := findWinner(state.Players)
				data := gin.H{"players": players, "winner": winner}
				html := state.renderToString("game-over.html", data)
				state.broadcastExcept(SSEEvent{Name: "game", Data: html}, sessionID)
				c.HTML(http.StatusOK, "game-over.html", data)
				return
			}

			state.PhaseNumber++
			state.PhaseWords = make([]string, len(state.AllPhaseWords))
			copy(state.PhaseWords, state.AllPhaseWords)

			data := gin.H{
				"phaseNumber": state.PhaseNumber,
				"phaseName":   phaseName(state.PhaseNumber),
				"phaseRules":  phaseRules(state.PhaseNumber),
				"activeName":  state.activePlayerName(),
			}
			html := state.renderToString("phase-transition.html", data)
			state.broadcastExcept(SSEEvent{Name: "game", Data: html}, sessionID)
			c.HTML(http.StatusOK, "phase-transition.html", data)
			return
		}

		data := gin.H{
			"phaseName":  phaseName(state.PhaseNumber),
			"phaseRules": phaseRules(state.PhaseNumber),
			"activeName": state.activePlayerName(),
		}
		html := state.renderToString("game-page.html", data)
		state.broadcastExcept(SSEEvent{Name: "game", Data: html}, sessionID)
		c.HTML(http.StatusOK, "game-page.html", data)
	})

	// /setup returns the current game state without modifying it (safe for page refresh)
	r.GET("/setup", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		sessionID, _ := c.Cookie("session_id")

		if !state.Started {
			c.HTML(http.StatusOK, "add-words.html", gin.H{})
			return
		}

		if state.RoundStarted {
			if sessionID == state.activePlayerID() {
				c.HTML(http.StatusOK, "active-view.html", gin.H{"word": state.CurrentWord})
			} else {
				c.HTML(http.StatusOK, "guesser-view.html", gin.H{"activeName": state.activePlayerName()})
			}
			return
		}

		data := gin.H{
			"phaseName":  phaseName(state.PhaseNumber),
			"phaseRules": phaseRules(state.PhaseNumber),
			"activeName": state.activePlayerName(),
		}
		c.HTML(http.StatusOK, "game-page.html", data)
	})

	r.GET("/restart", func(c *gin.Context) {
		state.mu.Lock()
		defer state.mu.Unlock()

		sessionID, _ := c.Cookie("session_id")
		state.reset()

		html := state.renderToString("add-words.html", gin.H{})
		state.broadcastExcept(SSEEvent{Name: "game", Data: html}, sessionID)
		clockHTML := state.renderToString("clock.tmpl", gin.H{"clear": true})
		state.broadcastAll(SSEEvent{Name: "clock", Data: clockHTML})
		c.HTML(http.StatusOK, "add-words.html", gin.H{})
	})

	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
