package commands

import (
	"krill-agg/root/managers/authorize"
	"krill-agg/root/managers/components"
	"krill-agg/root/managers/pages"
	"krill-agg/root/types"
	"os"
	"strings"
	"testing"
)

// every command that needs MANAGE or above is named mg-..., and nothing
// else is, so the manager commands sort together and read as such
func TestManagerCommandsArePrefixed(t *testing.T) {
	help, err := os.ReadFile("../../.command_help") // /help panics without its sections
	if err != nil {
		t.Fatal(err)
	}
	handles := map[string]CommandInfo{}
	var messages []types.MessageCallback
	bcp := &types.BotCommandPackage{Handles: &handles, MessageHandlers: &messages, Components: components.New(), Pages: pages.New(), HelpText: string(help)}
	for _, reg := range registers {
		reg(bcp)
	}
	if len(handles) < 10 {
		t.Fatalf("only %d commands registered", len(handles))
	}
	for key, h := range handles {
		name := h.DGInfo.Name
		if key != name {
			t.Errorf("handle %q registers as /%s", key, name)
		}
		manager := h.Metadata.MinTrustLevel >= authorize.MANAGE
		if prefixed := strings.HasPrefix(name, "mg-"); manager != prefixed {
			t.Errorf("/%s needs %s but mg- prefix is %v", name, authorize.Stringify(h.Metadata.MinTrustLevel), prefixed)
		}
	}
}
