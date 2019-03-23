package main

import (
	"github.com/ey-/cozgo/connection"
	"github.com/ey-/cozgo/messagetypes"
)

func main() {
	service := "172.31.1.1:5551"
	conn := connection.Connection{}
	conn.Init()
	messages := messagetypes.Messages{MessageTypeEnum: messagetypes.NewMessageTypes()}
	conn.Messages = messages
	conn.StartListen(service)
}
