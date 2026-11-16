package main

import (
	"errors"
	"fmt"
	"krill-agg/root/bot"
	"krill-agg/root/commands"
	"krill-agg/root/info/version"
	"krill-agg/root/managers/audit"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/scores"
	"krill-agg/root/managers/tracking"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// some globals
var (
	logger *audit.Log
)

var (
	LogFlag  = audit.LogFlag
	LogGroup = audit.LogGroup
)

const (
	DATA_PATH         = "./data/"
	LOG_PATH          = DATA_PATH + "logs/"
	DISCORD_AUTH_PATH = DATA_PATH + "discord_auth.db"
	SCORES_PATH       = DATA_PATH + "scores.db"
	TRACKING_PATH     = DATA_PATH + "tracking.db"
)

func setEnvironmentVars() (token string, mode string) {
	mode = strings.ToUpper(os.Getenv("MODE"))
	if mode == "" {
		log.Fatal(errors.New("you should provide a MODE"))
	}
	token = os.Getenv(mode + "_TOKENID")
	if token == "" {
		log.Fatal(fmt.Errorf("no token for MODE '%s' (canceled)", mode))
		return
	}
	return
}

func setupLogger(mode string) {
	lg, err := audit.New(LOG_PATH+mode, LogFlag.Default, audit.LogGroup.INIT)
	if err != nil {
		log.Fatal(fmt.Errorf("couldn't init logs: %v", err))
	}
	logger = lg
	logger.Add(fmt.Sprintf("Running in '%s' mode", mode), audit.LogGroup.INIT)
}

func getManAuth() *authorize.AuthManager {
	man, err := authorize.New(DISCORD_AUTH_PATH)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init auth manager: %v", err), audit.LogGroup.INIT)
		return nil
	}
	return man
}
func getManScores() *scores.ScoreManager {
	man, err := scores.New(SCORES_PATH)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init score manager: %v", err), audit.LogGroup.INIT)
		return nil
	}
	return man
}
func getManTracking() *tracking.TrackingManager {
	man, err := tracking.New(TRACKING_PATH)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init tracking manager: %v", err), audit.LogGroup.INIT)
		return nil
	}
	return man
}
func init() {
	log.SetFlags(log.Ldate) //only date no time
}
func persist() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func checkForUpdates() {
	logger.Add(version.Format(), audit.LogGroup.INIT)
	//
	fetch, err := version.TryForUpdates()
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to check for updates: %v", err), audit.LogGroup.INIT)
	}
	if fetch == nil {
		return
	}
	//
	logger.Add("Checking for updates...", audit.LogGroup.INIT)
	message, err := fetch()
	if err != nil {
		logger.Add(fmt.Sprintf("Failed to check for updates: %v", err), audit.LogGroup.INIT)
	} else if message != "" {
		logger.Add(message, audit.LogGroup.INIT)
	}
}
func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "--version" || os.Args[1] == "-v" {
			fmt.Printf("version: %s\nsource:  %s\n", version.Version(), version.Source())
			os.Exit(0)
		} else {
			fmt.Printf("Run with no commands to start the bot.")
			os.Exit(1)
		}
		return
	}

	token, mode := setEnvironmentVars()
	setupLogger(mode)
	defer logger.Close() // nolint

	checkForUpdates()

	commands := commands.Register(commands.RegisterReqs{
		Auth:       getManAuth(),
		Components: components.New(),
		Scores:     getManScores(),
		Tracking:   getManTracking(),
	})

	session, err := bot.Start(bot.StartInstructions{
		Token:    token,
		Commands: commands,
		Logger:   logger,
	})
	if err != nil {
		logger.Panic(fmt.Errorf("error creating Discord session: %v", err))
		return
	}
	defer bot.EndSession(session)

	persist()
}
