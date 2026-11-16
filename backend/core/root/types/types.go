package types

import (
	"krill-agg/root/managers/audit"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"

	"github.com/bwmarrin/discordgo"
)

type CommandMetadata struct {
	MinTrustLevel authorize.Rank
}
type EventCallback = func(s *discordgo.Session, i *discordgo.InteractionCreate) error

type CommandInfo struct {
	DGInfo   *discordgo.ApplicationCommand
	Metadata CommandMetadata
	Callback EventCallback
}

type BotCommandPackage struct {
	Handles    *map[string]CommandInfo
	Auth       *authorize.AuthManager
	Components *components.ComponentCallbackManager
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
