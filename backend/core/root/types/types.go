package types

import (
	"krill-agg/root/managers/audit"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/pages"
	"krill-agg/root/managers/scores"
	"krill-agg/root/managers/tracking"

	"github.com/bwmarrin/discordgo"
)

type CommandMetadata struct {
	MinTrustLevel authorize.Rank
}
type EventCallback = func(s *discordgo.Session, i *discordgo.InteractionCreate) error

// runs on every (non-bot) guild message. logMsg is only logged when non-empty,
// so handlers decide which messages are worth an audit line
type MessageCallback = func(s *discordgo.Session, m *discordgo.MessageCreate) (logMsg string, err error)

type CommandInfo struct {
	DGInfo   *discordgo.ApplicationCommand
	Metadata CommandMetadata
	Callback EventCallback
}

type BotCommandPackage struct {
	Handles         *map[string]CommandInfo
	MessageHandlers *[]MessageCallback
	Auth            *authorize.AuthManager
	Components      *components.ComponentCallbackManager
	Pages           *pages.Manager
	Leaderboard     *leaderboard.Manager
	Scores          *scores.ScoreManager
	Tracking        *tracking.TrackingManager
	DataDir         string // the shared data mount (also seen by dailygen)
}

type SessionCommandRegisters struct {
	Reference BotCommandPackage
}
type Session struct {
	DGSession *discordgo.Session
	Registers []*discordgo.ApplicationCommand
	//
	Logger  *audit.Log
	Package *BotCommandPackage
}

type StartInstructions struct {
	Token    string
	Commands *BotCommandPackage
	Logger   *audit.Log
}
