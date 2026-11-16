package main

import (
	"errors"
	"fmt"
	"krill-agg/root/bot"
	"krill-agg/root/commands"
	"krill-agg/root/commands/daily"
	"krill-agg/root/ctl"
	"krill-agg/root/info/version"
	"krill-agg/root/managers/audit"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/scores"
	"krill-agg/root/managers/tracking"
	"log"
	"net/url"
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

const CTL_USAGE = `usage: main ctl <command>
  daily   post today's report (fresh card) in every guild's main channel`

// the client half of the control socket: hands a command to the running bot
func runCtl(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, CTL_USAGE)
		return 2
	}
	switch args[0] {
	case "daily":
		if len(args) > 1 {
			fmt.Fprintln(os.Stderr, CTL_USAGE)
			return 2
		}
	default:
		fmt.Fprintln(os.Stderr, CTL_USAGE)
		return 2
	}
	out, err := ctl.Call(ctl.SOCKET, args[0], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if out != "" {
		fmt.Println(out)
	}
	return 0
}

func startCtl(session *bot.Session) *ctl.Server {
	srv, err := ctl.Serve(ctl.SOCKET, map[string]ctl.Handler{
		"daily": daily.CtlDaily(session.DGSession, session.Package, func(msg string) {
			logger.Add(msg, audit.LogGroup.CTL)
		}),
	}, func(cmd string, args url.Values, out string, err error) {
		// failures in full; a success only by its summary line (the caller gets the rest)
		if err != nil {
			logger.Warn(fmt.Sprintf("ctl %s: %v", cmd, err), audit.LogGroup.CTL)
			return
		}
		summary, _, _ := strings.Cut(out, "\n")
		logger.Add(fmt.Sprintf("ctl %s: %s", cmd, summary), audit.LogGroup.CTL)
	})
	if err != nil {
		// the bot still works; only scheduled posts are lost
		logger.Warn(fmt.Sprintf("Couldn't open control socket: %v", err), audit.LogGroup.INIT, audit.LogGroup.CTL)
		return nil
	}
	logger.Add("Control socket at "+ctl.SOCKET, audit.LogGroup.INIT, audit.LogGroup.CTL)
	return srv
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("version: %s\nsource:  %s\n", version.Version(), version.Source())
			os.Exit(0)
		case "ctl":
			os.Exit(runCtl(os.Args[2:]))
		default:
			fmt.Printf("Run with no commands to start the bot, or `ctl` to drive a running one.")
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
		DataDir:    DATA_PATH,
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
	if srv := startCtl(session); srv != nil {
		defer srv.Close() //nolint
	}

	persist()
}
