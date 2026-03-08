package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"fishbowl/game"

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

	game_state := gameState.new()
	words := []string{}
	started := false
	round_started := false
	round := []string{}
	var start_time *time.Time = nil
	round_length := 30 * time.Second

	r.GET("/", func(c *gin.Context) {
		c.File("templates/index.html")
	})
	r.GET("/game", func(c *gin.Context) {
		c.File("templates/game-page.html")
	})

	r.POST("/words", func(ctx *gin.Context) {
		if started {
			ctx.String(http.StatusLocked, "Game has already started")
			return
		}
		word := ctx.PostForm("word")
		words = append(words, word)
		logger.Print("Phrases: ", words)
		ctx.String(http.StatusOK, "added word "+word+"\n")
	})

	r.GET("/words/count", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, len(words))
	})

	r.GET("/words/random", func(ctx *gin.Context) {
		if !started || !round_started {
			ctx.HTML(http.StatusOK, "word.tmpl", gin.H{"word": "Finished!"})
			return
		}

		i := rand.Intn(len(round))
		pick := round[i]

		if i == len(round) {
			round = round[:i]
		} else {
			round = append(round[:i], round[i+1:]...)
		}

		if len(round) == 0 {
			round_started = false
			ctx.HTML(http.StatusOK, "word.tmpl", gin.H{"word": "Finished!"})
		} else {
			ctx.HTML(http.StatusOK, "word.tmpl", gin.H{"word": pick})
		}
	})

	r.GET("/time", func(ctx *gin.Context) {
		if start_time == nil {
			ctx.String(http.StatusConflict, "Game has not started")
			return
		}
		remaining_time := round_length - time.Since(*start_time)
		if remaining_time <= 0 {
			ctx.HTML(http.StatusOK, "clock-stopped.tmpl", gin.H{})
			return
		}
		ctx.HTML(http.StatusOK, "clock.tmpl", gin.H{"clock_id": "round-clock", "time": fmt.Sprintf("%.2f", remaining_time.Seconds())})
	})

	r.GET("/start-game", func(ctx *gin.Context) {
		started = true
		ctx.Redirect(http.StatusPermanentRedirect, "game-page.html")
		// ctx.HTML(http.StatusOK, "game-page.html", gin.H{})
	})

	r.GET("/start-round", func(ctx *gin.Context) {
		if !started {
			ctx.String(http.StatusNotAcceptable, "Game has not started yet")
			return
		}
		current_time := time.Now()
		start_time = &current_time
		round_started = true
		round = words
		ctx.HTML(http.StatusOK, "draw-word.html", gin.H{})
	})

	r.GET("/restart", func(ctx *gin.Context) {
		started = false
		start_time = nil
		ctx.HTML(http.StatusOK, "add-words.html", gin.H{})
	})

	// Start server on port 8080 (default)
	// Server will listen on 0.0.0.0:8080 (localhost:8080 on Windows)
	if err := r.Run(); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
