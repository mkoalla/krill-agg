package commands

import (
	"krill-agg/root/commands/backfill"
	"krill-agg/root/commands/channels"
	"krill-agg/root/commands/daily"
	"krill-agg/root/commands/day"
	"krill-agg/root/commands/modrank"
	"krill-agg/root/commands/ping"
	"krill-agg/root/commands/rank"
	"krill-agg/root/commands/score"
	"krill-agg/root/commands/stats"
	"krill-agg/root/commands/submit"
	"krill-agg/root/commands/version"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/scores"
	"krill-agg/root/managers/tracking"
	"krill-agg/root/types"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

type RegisterReqs struct {
	Auth       *authorize.AuthManager
	Components *components.ComponentCallbackManager
	Scores     *scores.ScoreManager
	Tracking   *tracking.TrackingManager
}

var registers = []func(*types.BotCommandPackage){
	backfill.Register,
	channels.Register,
	daily.Register,
	day.Register,
	modrank.Register,
	ping.Register,
	rank.Register,
	score.Register,
	stats.Register,
	submit.Register,
	version.Register,
}

func Register(reqs RegisterReqs) *types.BotCommandPackage {
	handles := make(map[string]CommandInfo)
	messageHandlers := make([]types.MessageCallback, 0)
	bcp := types.BotCommandPackage{
		Handles:         &handles,
		MessageHandlers: &messageHandlers,
		Auth:            reqs.Auth,
		Components:      reqs.Components,
		Scores:          reqs.Scores,
		Tracking:        reqs.Tracking,
	}

	for _, reg := range registers {
		reg(&bcp)
	}
	return &bcp
}
