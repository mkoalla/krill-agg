package commands

import (
	"krill-agg/root/commands/modrank"
	"krill-agg/root/commands/ping"
	"krill-agg/root/commands/rank"
	"krill-agg/root/commands/version"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/types"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

type RegisterReqs struct {
	Auth       *authorize.AuthManager
	Components *components.ComponentCallbackManager
}

var registers = []func(*types.BotCommandPackage){
	modrank.Register,
	ping.Register,
	rank.Register,
	version.Register,
}

func Register(reqs RegisterReqs) *types.BotCommandPackage {
	handles := make(map[string]CommandInfo)
	bcp := types.BotCommandPackage{
		Handles:    &handles,
		Auth:       reqs.Auth,
		Components: reqs.Components,
	}

	for _, reg := range registers {
		reg(&bcp)
	}
	return &bcp
}
