package main

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"krill-agg/root/bot"
	"krill-agg/root/commands"
	"krill-agg/root/commands/daily"
	"krill-agg/root/ctl"
	"krill-agg/root/info/dailygen"
	"krill-agg/root/info/version"
	"krill-agg/root/managers/audit"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/distributions"
	"krill-agg/root/managers/krills"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/pages"
	"krill-agg/root/managers/questions"
	"krill-agg/root/managers/scores"
	"krill-agg/root/managers/tracking"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// /help's entries (see the help command), built into the binary so the
// release image needs no file beside it
//
//go:embed .command_help
var commandHelp string

// some globals
var (
	logger *audit.Log
)

var (
	LogFlag  = audit.LogFlag
	LogGroup = audit.LogGroup
)

const (
	DATA_PATH          = "./data/"
	LOG_PATH           = DATA_PATH + "logs/"
	DISCORD_AUTH_PATH  = DATA_PATH + "discord_auth.db"
	SCORES_PATH        = DATA_PATH + "scores.db"
	TRACKING_PATH      = DATA_PATH + "tracking.db"
	LEADERBOARD_PATH   = DATA_PATH + "leaderboard.db"
	QUESTIONS_PATH     = DATA_PATH + "questions.db"
	DISTRIBUTIONS_PATH = DATA_PATH + "distributions.db"
	KRILLS_PATH        = DATA_PATH + "krills.db"
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
func getManLeaderboard(scores *scores.ScoreManager) *leaderboard.Manager {
	man, err := leaderboard.New(LEADERBOARD_PATH, scores)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init leaderboard manager: %v", err), audit.LogGroup.INIT)
		return nil
	}
	return man
}
func getManQuestions() *questions.Manager {
	man, err := questions.New(QUESTIONS_PATH)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init questions manager: %v", err), audit.LogGroup.INIT)
		return nil
	}
	return man
}
func getManDistributions() *distributions.Manager {
	man, err := distributions.New(DISTRIBUTIONS_PATH)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init distributions manager: %v", err), audit.LogGroup.INIT)
		return nil
	}
	return man
}
func getManKrills() *krills.Manager {
	man, err := krills.New(KRILLS_PATH)
	if err != nil {
		logger.Panic(fmt.Sprintf("Couldn't init krills manager: %v", err), audit.LogGroup.INIT)
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

// `main check`: this binary's version, then whether a newer release is out.
// standalone, no bot or socket needed
func runCheck() int {
	fmt.Printf("krill-agg %s (%s)\n", version.Version(), version.Source())
	fetch, err := version.TryForUpdates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't check for updates: %v\n", err)
		return 1
	}
	if fetch == nil {
		fmt.Println("Local build: no releases to compare against.")
		return 0
	}
	message, err := fetch()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to check for updates: %v\n", err)
		return 1
	}
	fmt.Println(message)
	return 0
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

// bytes under dir, counting every file
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// past this many MB of rendered cards, launch logs a warning
const DEFAULT_WARN_SIZE_MB = 1024

// once at launch: warns if the rendered cards (data/dailygen) have grown past
// WARN_SIZE_MB megabytes (DEFAULT_WARN_SIZE_MB when unset)
func checkCardsSize() {
	raw := os.Getenv("WARN_SIZE_MB")
	if raw == "" {
		raw = strconv.Itoa(DEFAULT_WARN_SIZE_MB)
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		logger.Warn(fmt.Sprintf("WARN_SIZE_MB=%q isn't a positive whole number; not checking card storage", raw), audit.LogGroup.INIT)
		return
	}
	dir := filepath.Join(DATA_PATH, dailygen.DIR)
	size, err := dirSize(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		logger.Warn(fmt.Sprintf("Couldn't measure %s: %v", dir, err), audit.LogGroup.INIT)
		return
	}
	if mb := size / (1 << 20); mb > int64(limit) {
		logger.Warn(fmt.Sprintf("%s is %d MB, over WARN_SIZE_MB=%d", dir, mb, limit), audit.LogGroup.INIT)
	}
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
		case "check":
			os.Exit(runCheck())
		default:
			fmt.Println("Run with no commands to start the bot, `check` to look for updates, or `ctl` to drive a running one.")
			os.Exit(1)
		}
		return
	}

	token, mode := setEnvironmentVars()
	setupLogger(mode)
	defer logger.Close() // nolint

	checkForUpdates()
	checkCardsSize()

	scoreMan := getManScores()
	commands := commands.Register(commands.RegisterReqs{
		Auth:          getManAuth(),
		Components:    components.New(),
		Pages:         pages.New(),
		Leaderboard:   getManLeaderboard(scoreMan),
		Questions:     getManQuestions(),
		Distributions: getManDistributions(),
		Krills:        getManKrills(),
		Scores:        scoreMan,
		Tracking:      getManTracking(),
		DataDir:       DATA_PATH,
		HelpText:      commandHelp,
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
