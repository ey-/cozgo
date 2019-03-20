package main

import (
	//"io"
	//"github.com/jfreymuth/oggvorbis"
	"github.com/ey-/cozgo/connection"
	//    "encoding/gob"
)

func main() {
	service := "172.31.1.1:5551"
	connection.Init()
	connection.StartListen(service)
}
