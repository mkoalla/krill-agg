package commands

import (
	"krill-agg/root/commands/backfill"
	"krill-agg/root/commands/channels"
	"krill-agg/root/commands/daily"
	"krill-agg/root/commands/day"
	"krill-agg/root/commands/drop"
	"krill-agg/root/commands/help"
	"krill-agg/root/commands/krill"
	"krill-agg/root/commands/modrank"
	"krill-agg/root/commands/ping"
	"krill-agg/root/commands/quantile"
	"krill-agg/root/commands/rank"
	"krill-agg/root/commands/score"
	"krill-agg/root/commands/stats"
	"krill-agg/root/commands/submit"
	"krill-agg/root/commands/top"
	"krill-agg/root/commands/version"
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/confirm"
	"krill-agg/root/managers/distributions"
	"krill-agg/root/managers/krills"
	"krill-agg/root/managers/leaderboard"
	"krill-agg/root/managers/pages"
	"krill-agg/root/managers/questions"
	"krill-agg/root/managers/scores"
	"krill-agg/root/managers/tracking"
	"krill-agg/root/types"
)

type CommandInfo = types.CommandInfo
type EventCallback = types.EventCallback

type RegisterReqs struct {
	Auth          *authorize.AuthManager
	Components    *components.ComponentCallbackManager
	Pages         *pages.Manager
	Leaderboard   *leaderboard.Manager
	Questions     *questions.Manager
	Distributions *distributions.Manager
	Krills        *krills.Manager
	Scores        *scores.ScoreManager
	Tracking      *tracking.TrackingManager
	DataDir       string
	HelpText      string
}

var registers = []func(*types.BotCommandPackage){
	backfill.Register,
	channels.Register,
	daily.Register,
	day.Register,
	drop.Register,
	help.Register,
	krill.Register,
	modrank.Register,
	ping.Register,
	quantile.Register,
	rank.Register,
	score.Register,
	stats.Register,
	submit.Register,
	top.Register,
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
		Pages:           reqs.Pages,
		Leaderboard:     reqs.Leaderboard,
		Questions:       reqs.Questions,
		Distributions:   reqs.Distributions,
		Krills:          reqs.Krills,
		Scores:          reqs.Scores,
		Tracking:        reqs.Tracking,
		DataDir:         reqs.DataDir,
		HelpText:        reqs.HelpText,
	}

	// flips reach the pages manager through the components' prefix routing
	bcp.Pages.Register(bcp.Components.AddPrefixHandler, func(user, command string) bool {
		handle, ok := (*bcp.Handles)[command]
		return ok && bcp.Auth.CanUse(user, handle.Metadata.MinTrustLevel)
	})
	// as do confirm prompts' Confirm/Cancel clicks
	confirm.Register(bcp.Components.AddPrefixHandler)
	for _, reg := range registers {
		reg(&bcp)
	}
	return &bcp
}
