package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"fishbowl.deligator.co.uk/game"

	"github.com/gin-gonic/gin"
)

func main() {
	// Create a Gin router with default middleware (logger and recovery)
	r := gin.New()
	r.LoadHTMLGlob("templates/*")
	r.Static("/static", "./static")
	r.Use(gin.Recovery())
	// Don't log time
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{
			"/time",
		},
	}))

	logger := log.Default()

	var state game.State

	r.GET("/", func(c *gin.Context) {
		c.File("templates/index.html")
	})
	r.GET("/game", func(c *gin.Context) {
		c.File("templates/game-page.html")
	})

	r.POST("/words", func(ctx *gin.Context) {
		if state.Started {
			ctx.String(http.StatusLocked, "Game has already started")
			return
		}
		word := ctx.PostForm("word")
		state.Words = append(state.Words, word)
		logger.Print("Phrases: ", state.Words)
		ctx.String(http.StatusOK, "added word "+word+"\n")
	})

	r.GET("/words/count", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, len(state.Words))
	})

	r.GET("/words/random", func(ctx *gin.Context) {
		if !state.Started || !state.Round_started {
			ctx.HTML(http.StatusOK, "word.tmpl", gin.H{"word": "Finished!"})
			return
		}

		i := rand.Intn(len(state.Remaining_words))
		pick := state.Remaining_words[i]

		if i == len(state.Remaining_words) {
			state.Remaining_words = state.Remaining_words[:i]
		} else {
			state.Remaining_words = append(state.Remaining_words[:i], state.Remaining_words[i+1:]...)
		}

		if len(state.Remaining_words) == 0 {
			state.Round_started = false
			ctx.HTML(http.StatusOK, "word.tmpl", gin.H{"word": "Finished!"})
		} else {
			ctx.HTML(http.StatusOK, "word.tmpl", gin.H{"word": pick})
		}
	})

	r.GET("/time", func(ctx *gin.Context) {
		if !state.Started {
			ctx.String(http.StatusConflict, "Game has not started")
			return
		}
		remaining_time := state.Round_length - time.Since(state.Start_time)
		if remaining_time <= 0 {
			ctx.HTML(http.StatusOK, "clock-stopped.tmpl", gin.H{})
			return
		}
		ctx.HTML(http.StatusOK, "clock.tmpl", gin.H{"clock_id": "round-clock", "time": fmt.Sprintf("%.2f", remaining_time.Seconds())})
	})

	r.GET("/start-game", func(ctx *gin.Context) {
		state = game.New(
			time.Now(), 30*time.Second,
		)
		state.Started = true
		ctx.Redirect(http.StatusPermanentRedirect, "game")
	})

	r.GET("/start-round", func(ctx *gin.Context) {
		logger.Print(state)
		if !state.Started {
			ctx.String(http.StatusNotAcceptable, "Game has not started yet")
			return
		}
		state.Start_time = time.Now()
		state.Round_started = true
		state.Remaining_words = state.Words
		ctx.HTML(http.StatusOK, "draw-word.html", gin.H{})
	})

	r.GET("/restart", func(ctx *gin.Context) {
		state.Started = false
		state.Start_time = time.Time{}
		ctx.HTML(http.StatusOK, "add-words.html", gin.H{})
	})

	// Start server on port 8080 (default)
	// Server will listen on 0.0.0.0:8080 (localhost:8080 on Windows)
	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
