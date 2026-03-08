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

type State struct {
	Start_time      time.Time
	Round_length    time.Duration
	Words           []string
	Remaining_words []string
	Started         bool
	Round_started   bool
	Round           round
}

func New(start_time time.Time, round_length time.Duration) State {
	return State{
		Start_time:      start_time,
		Round_length:    round_length,
		Words:           []string{},
		Remaining_words: []string{},
		Started:         false,
		Round_started:   false,
		Round:           articulate,
	}
}
