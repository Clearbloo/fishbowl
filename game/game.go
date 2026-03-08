package game

import (
	"time"
)

type round int

const (
	articulate round = iota
	charades         = iota
	spade            = iota
)

type gameState struct {
	start_time      time.Time
	round_length    time.Duration
	words           []string
	remaining_words []string
	started         bool
	round_num       round
}

func (g gameState) New(start_time time.Time, round_length time.Duration) gameState {
	return gameState{
		start_time:      start_time,
		round_length:    round_length,
		words:           []string{},
		remaining_words: []string{},
		started:         false,
		round_num:       articulate,
	}
}
