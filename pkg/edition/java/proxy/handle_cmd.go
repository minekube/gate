package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

func handleCommand(protocol proto.Protocol) {
	if protocol.GreaterEqual(version.Minecraft_1_19_3) {
		handleSessionCommand()
	} else if protocol.GreaterEqual(version.Minecraft_1_19) {
		handleKeyedCommand()
	} else {
		handleLegacyCommand()
	}
}

func handleSessionCommand() {

}

func handleKeyedCommand() {

}

func handleLegacyCommand() {

}
