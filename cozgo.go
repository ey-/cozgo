package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"time"
	//"io"
	//"github.com/jfreymuth/oggvorbis"
	"github.com/pkg/term"
	"gocv.io/x/gocv"
	//    "encoding/gob"
)

type firstpack struct {
	startBytes [7]byte
	Content    [7]byte
}

type tickPackage struct {
	Header            Header
	RandomPart        [7]byte
	Sixtyfour         byte
	IteratingSend     [4]byte
	IteratingReceived [5]byte
}

// Resp type 0b
type TockAnswer struct {
	RandomPart        [7]byte
	Sixtyfour         byte
	IteratingSend     [4]byte
	IteratingReceived [5]byte
}

// Resp type 0b
type Header struct {
	Type     byte
	From     [2]byte
	FromNext [2]byte
	To       [2]byte
}

type LiftUp struct {
	Type        byte
	Length      uint16
	MessageType uint16
	Pos         uint16
}

var messageTypeEnum = newMessageTypes()

func newMessageTypes() *messageTypes {
	return &messageTypes{
		FromRobot:      0x09,
		InitialRequest: 0x01,
		Tick:           0x0b,
		Unknown7:       0x07,
		Unknown4:       0x04,
	}
}

var tickMap map[string]int
var connectionReady bool
var resetSend bool
var cameraStreamActivated bool
var cameraReady bool
var readyForTickTock bool
var ToRobotHeader uint16
var FromUs uint16
var a int
var liftPos = liftPosInit()
var tickPack = newtickPack()

func newtickPack() *tickPackage {
	header := Header{
		Type:     0x0b,
		From:     [2]byte{0, 0},
		FromNext: [2]byte{0, 0},
		To:       [2]byte{0, 0},
	}
	return &tickPackage{
		Header:            header,
		RandomPart:        generateRandomPart(),
		Sixtyfour:         64,
		IteratingSend:     [4]byte{0, 0, 0, 0},
		IteratingReceived: [5]byte{0, 0, 0, 0, 0},
	}
}

func liftPosInit() *LiftUp {
	return &LiftUp{
		Type:        4,
		Length:      uint16(4),
		MessageType: uint16(148), //0x94, 0x00
		Pos:         uint16(0),
	}
}

func generateRandomPart() [7]byte {
	var randomValue [7]byte
	token := make([]byte, 7)
	rand.Read(token)
	copy(randomValue[:], token)
	return randomValue
}

type messageTypes struct {
	FromRobot      byte
	InitialRequest byte
	Tick           byte
	Unknown7       byte
	Unknown4       byte
}

func firstBytes() [7]byte {
	var arr [7]byte
	copy(arr[:], []byte("COZ\x03RE\x01"))
	return arr
}

func firstpacket() firstpack {
	packet := firstpack{}
	packet.startBytes = firstBytes()
	content := []byte("\x01\x01\x00\x01\x00\x00\x00")
	copy(packet.Content[:], content)
	return packet
}

func packet(Content []byte) *bytes.Buffer {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, firstBytes())
	binary.Write(buf, binary.LittleEndian, Content)
	return buf
}

func getMessage(bufInput []byte) []byte {
	// Strip the first 7 bytes
	reader := bytes.NewReader(bufInput)
	message := make([]byte, 2048)
	reader.ReadAt(message, 7)
	return message
}

func sendTick(connection *net.UDPConn, remAddr *net.UDPAddr) {
	if !readyForTickTock {
		return
	}
	buf := &bytes.Buffer{}
	tickPack.RandomPart = generateRandomPart()
	tickPack.IteratingSend[0]++
	if tickPack.IteratingSend[0] == 0 {
		tickPack.IteratingSend[1]++
		if tickPack.IteratingSend[1] == 0 {
			tickPack.IteratingSend[2]++
			if tickPack.IteratingSend[2] == 0 {
				tickPack.IteratingSend[3]++
			}
		}
	}

	head := make([]byte, 2)
	binary.LittleEndian.PutUint16(head, ToRobotHeader)
	copy(tickPack.Header.To[:], head)

	tickBuf := &bytes.Buffer{}
	binary.Write(tickBuf, binary.LittleEndian, tickPack)
	tickMessage := packet(tickBuf.Bytes())
	err := binary.Write(buf, binary.LittleEndian, tickMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	tickMap[string(tickPack.RandomPart[:])] = 1

	if err != nil {

	}
	go send(connection, buf, remAddr)
}

func getReadyMessage() *bytes.Buffer {
	FromUs = FromUs + uint16(1)
	header := buildHeader(0x04, uint16(0))
	messageBuffer := &bytes.Buffer{}
	// Signal "accepted" or "ready"? Causes robot to return response with 30 length
	// After which we can send the reset command to the robot.
	//readyMessage := []byte("\x00")
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, []byte{0x25})
	ready := packet(messageBuffer.Bytes())
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.LittleEndian, ready.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	// fmt.Println("%s", hex.EncodeToString(buf.Bytes()))
	return buf
}

func getSecondReadyMessage() *bytes.Buffer {
	header := buildHeader(0x04, uint16(0))
	messageBuffer := &bytes.Buffer{}
	readyMessage := []byte("\x64\xff\xff")
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, readyMessage)
	ready := packet(messageBuffer.Bytes())
	buf := &bytes.Buffer{}
	// Signal "accepted" or "ready"? Causes robot to return response with 30 length
	// After which we can send the reset command to the robot.
	err := binary.Write(buf, binary.LittleEndian, ready.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	return buf
}

func main() {
	tickMap = make(map[string]int)
	readyForTickTock = false
	connectionReady = false
	resetSend = false
	cameraStreamActivated = false
	cameraReady = false
	a = 0
	service := "172.31.1.1:5551"

	// Resolving Address
	RemoteAddr, err := net.ResolveUDPAddr("udp", service)
	LocalAddr, err := net.ResolveUDPAddr("udp", ":0")

	conn, err := net.ListenUDP("udp", LocalAddr)

	// Exit if some error occured
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	listen(conn, RemoteAddr)
}

func doTick(connection *net.UDPConn, remAddr *net.UDPAddr) {
	for {
		<-time.After(50 * time.Millisecond)
		go sendTick(connection, remAddr)
	}
}

func buildHeader(typeToUse byte, addFrom uint16) Header {
	header := Header{
		Type:     typeToUse,
		From:     [2]byte{0, 0},
		FromNext: [2]byte{0, 0},
		To:       [2]byte{0, 0},
	}
	toRobot := make([]byte, 2)
	from := make([]byte, 2)
	fromNext := make([]byte, 2)
	binary.LittleEndian.PutUint16(toRobot, ToRobotHeader)
	binary.LittleEndian.PutUint16(from, FromUs)
	binary.LittleEndian.PutUint16(fromNext, FromUs+addFrom)
	FromUs = FromUs + addFrom
	copy(header.To[:], toRobot)
	copy(header.FromNext[:], fromNext)
	copy(header.From[:], from)
	return header
}

func resetCommand() *bytes.Buffer {
	FromUs = FromUs + uint16(1)
	header := buildHeader(0x07, uint16(0))
	messageBuffer := &bytes.Buffer{}
	// 4b 7ae100000000a0c1 (check the meaning of second part)
	reset := []byte("\x04\x09\x00\x4b\x7a\xe1\x00\x00\x00\x00\xa0\xc1")
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, reset)

	// 9f, when this is send, robot sends 05 - f1 answers too.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x01\x00\x9f"))

	// 4c0104, with this send, there are no 05 - f3 answers anymore.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x03\x00\x4c\x01\x04"))

	// 45000000000000000001000000000000000000000000000080
	// With this, we start getting 05 - f0 answers.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x19\x00\x45\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x80"))

	// 5700000000000001, effect not visible.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x08\x00\x57\x00\x00\x00\x00\x00\x00\x01"))

	// a0 f9639fb3b03a45b19fb46f5d3c441c28, effect not visible.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x11\x00\xa0"))
	binary.Write(messageBuffer, binary.LittleEndian, generateRandomBytes(16))

	// 8000000000, effect not visible.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x05\x00\x80\x00\x00\x00\x00"))

	resetMessage := packet(messageBuffer.Bytes())
	buf := &bytes.Buffer{}
	// Signal "accepted" or "ready"? Causes robot to return response with 30 length
	// After which we can send the reset command to the robot.
	err := binary.Write(buf, binary.LittleEndian, resetMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	return buf
}

func generateRandomBytes(length int) []byte {
	token := make([]byte, length)
	rand.Read(token)
	return token
}

func toLeft_old(connection *net.UDPConn, remAddr *net.UDPAddr) {

	buf := &bytes.Buffer{}
	m8f := []byte{0x04, 0x01, 0x00, 0x8f}
	//firstGesicht := []byte("\x04\x22\x00\x97\x01\x00\x16\xa4\xb1\xa0\xa0\xb9\x41\x9c\xc1\x5c\xa0\xb9\x40\xa4\xb1\x06\xa4\xb1\x40\xa0\xb9\x40\x9c\xc1\x5d\xa0\xb9\x40\xa4\xb1\x16")
	//firstGesicht := []byte{0x04,0x22,0x00, 0x97, 0x1f, 0x00, 0x13, 0xa0, 0xb6, 0x40, 0x9c, 0xbe, 0x9c, 0x98, 0xc6, 0x5d, 0x9c, 0xbe, 0x40, 0xa0, 0xb6, 0x05, 0xa4, 0xae, 0x40, 0xa0, 0xb6, 0xa0, 0x9c, 0xbe, 0x5d, 0xa0, 0xb6, 0x40, 0xa4, 0xae, 0x1b}
	//secondGesicht := []byte{0x04,0x22,0x00, 0x97, 0x1f, 0x00, 0x13, 0xa4, 0xb2, 0xa0, 0xa0, 0xba, 0x40, 0x9c, 0xc2, 0x5e, 0xa0, 0xba, 0x40, 0xa4, 0xb2, 0x03, 0xa8, 0xaa, 0x40, 0xa4, 0xb2, 0x40, 0xa0, 0xba, 0x5d, 0xa4, 0xb2, 0x40, 0xa8, 0xaa, 0x1b}
	//thirdGesicht := []byte{0x04, 0x24, 0x00, 0x97, 0x21, 0x00, 0x12, 0xa8, 0xb2, 0x41, 0xa4, 0xba, 0x98, 0xa0, 0xc2, 0x5c, 0xa4, 0xba, 0x40, 0xa8, 0xb2, 0x40, 0x03, 0xac, 0xaa, 0x40, 0xa8, 0xb2, 0x41, 0xa4, 0xba, 0x5b, 0xa8, 0xb2, 0x40, 0xac, 0xaa, 0x40, 0x1c}
	fourth := []byte{0x04, 0x29, 0x00, 0x97, 0x26, 0x00, 0x1a, 0xa0, 0xad, 0xa8, 0x9c, 0xb5, 0x40, 0x98, 0xbd, 0x40, 0x98, 0xc1, 0x5b, 0x98, 0xbd, 0xa0, 0x9c, 0xb5, 0x40, 0x06, 0x9c, 0xb5, 0xa4, 0x98, 0xbd, 0x40, 0x94, 0xc5, 0x9c, 0x90, 0xcd, 0x5d, 0x94, 0xc5, 0x40, 0x98, 0xbd, 0x12}
	//toLeft := []byte("\x04\x01\x00\x8f\x04\x22\x00\x97\x01\x00\x16\xa4\xb1\xa0\xa0\xb9\x41\x9c\xc1\x5c\xa0\xb9\x40\xa4\xb1\x06\xa4\xb1\x40\xa0\xb9\x40\x9c\xc1\x5d\xa0\xb9\x40\xa4\xb1\x16")
	//toLeft := []byte("\x04\x01\x00\x8f\x04\x22\x00\x97\x01\x00\x16\xa4\xb1\xa0\xa0\xb9\x41\x9c\xc1\x5c\xa0\xb9\x40\xa4\xb1\x06\xa4\xb1\x40\xa0\xb9\x40\x9c\xc1\x5d\xa0\xb9\x40\xa4\xb1\x16")
	//toLeft := []byte("x04\x01\x00\x8f\x04\x23\x00\x97\x20\x00\x13\xa0\xad\xa8\x9c\xb5\x41\x98\xbd\x5e\x9c\xb5\x40\xa0\xad\x05\xa0\xad\x40\x9c\xb5\x40\x98\xbd\x5d\x9c\xb5\x40\xa0\xad\x40\x17\x04\x01\x00\x8f\x04\x1a\x00\x97\x17\x00\x10\xa8\x9d\xb0\xa4\xa5\x41\xa0\xad\x64\xa4\xa5\x41\xa8\x9d\x40\xa4\xa5\x67\xa8\x9d\x40\x14")
	//toLeft := []byte("\x04\x01\x00\x8f\x04\x0d\x00\x81\x00\x20\x18\x00\x00\x04\x00\x00\x00\x00\x00\x00")
	audio := []byte{0x04, 0xe9, 0x02, 0x8e, 0x15, 0x87, 0x09, 0x0b, 0x00, 0x0d, 0x9e, 0x20, 0x98, 0x07, 0x08, 0x8c, 0x25, 0x94, 0x27, 0x0b, 0x8b, 0x0f, 0x8f, 0x0c, 0x11, 0x93, 0x82, 0x8a, 0x99, 0x88, 0x97, 0xa3, 0x80, 0xa4, 0x13, 0x8d, 0x83, 0x01, 0x87, 0x04, 0x12, 0x0c, 0x01, 0x25, 0xa2, 0x22, 0x07, 0x95, 0x25, 0xb1, 0x29, 0x89, 0x20, 0x22, 0x92, 0x2e, 0xa4, 0x25, 0x96, 0x8a, 0x8a, 0xa7, 0x1c, 0xa3, 0x30, 0x87, 0x16, 0x16, 0x8b, 0x1f, 0xab, 0x01, 0xab, 0x9d, 0x89, 0xa8, 0x1c, 0xa5, 0x95, 0x02, 0x8e, 0x24, 0x9a, 0x21, 0x8f, 0x81, 0x06, 0x80, 0x98, 0x80, 0x86, 0x89, 0x0e, 0x81, 0x05, 0x81, 0x87, 0x1c, 0x06, 0x18, 0x0b, 0x0a, 0x01, 0x20, 0x8b, 0x23, 0x90, 0x00, 0x26, 0xa7, 0x30, 0x98, 0x89, 0x21, 0x85, 0x2c, 0x12, 0x23, 0x17, 0x85, 0x05, 0x8c, 0x93, 0xa7, 0x8f, 0xaa, 0x01, 0x87, 0xaa, 0x0b, 0xaf, 0x86, 0x04, 0xa5, 0x23, 0xa5, 0x0d, 0x08, 0x94, 0x0f, 0x91, 0x8a, 0x00, 0x01, 0x15, 0x04, 0x0f, 0x02, 0x08, 0x04, 0x11, 0x81, 0x07, 0x14, 0x83, 0x1b, 0x88, 0x05, 0x9c, 0x91, 0x03, 0xa2, 0x19, 0x86, 0x8a, 0x17, 0x06, 0x08, 0x9d, 0x26, 0xa5, 0x1f, 0x0c, 0x07, 0x22, 0x87, 0x81, 0x1d, 0xa5, 0x25, 0x8d, 0x18, 0x0d, 0x8d, 0x18, 0x91, 0x93, 0x08, 0xae, 0x14, 0x95, 0x8a, 0x17, 0x9d, 0x14, 0x8f, 0x9d, 0x10, 0xa0, 0x86, 0x0a, 0x96, 0x29, 0xa7, 0x24, 0x82, 0x86, 0x12, 0x06, 0x94, 0x2b, 0xa9, 0x2b, 0xa6, 0x0f, 0x82, 0x92, 0x08, 0x1a, 0x9e, 0x2b, 0xad, 0x24, 0xa7, 0x8c, 0x84, 0x9b, 0x8e, 0x23, 0xa5, 0x2b, 0x97, 0x20, 0x01, 0x8d, 0x19, 0x86, 0x89, 0x26, 0x9b, 0x2a, 0x8c, 0x1d, 0x14, 0x8c, 0x14, 0x85, 0x94, 0x1e, 0xa7, 0x12, 0x8f, 0x8c, 0x0b, 0xa2, 0x06, 0x85, 0xa6, 0x10, 0xa3, 0x84, 0x88, 0xa9, 0x1f, 0xa8, 0x08, 0x0b, 0x8e, 0x20, 0x0d, 0x02, 0x26, 0xa5, 0x2c, 0xa7, 0x16, 0x0e, 0x08, 0x14, 0x25, 0x8a, 0x30, 0xac, 0x2c, 0x9b, 0x83, 0x0f, 0x00, 0x94, 0x25, 0xa8, 0x25, 0xa8, 0x0a, 0x91, 0x97, 0x82, 0x85, 0x9f, 0x20, 0xa7, 0x24, 0xa4, 0x0b, 0x09, 0x8c, 0x09, 0x08, 0xa0, 0x1c, 0xa0, 0x0d, 0x8f, 0x83, 0x06, 0x85, 0x83, 0x02, 0x87, 0x09, 0x84, 0x05, 0x00, 0x00, 0x0b, 0x07, 0x0b, 0x0d, 0x0a, 0x0d, 0x0a, 0x03, 0x04, 0x00, 0x85, 0x85, 0x88, 0x83, 0x00, 0x02, 0x06, 0x08, 0x03, 0x80, 0x8a, 0x8d, 0x8b, 0x8b, 0x88, 0x85, 0x84, 0x02, 0x00, 0x00, 0x81, 0x81, 0x82, 0x00, 0x84, 0x02, 0x04, 0x06, 0x08, 0x0b, 0x0a, 0x0a, 0x05, 0x04, 0x02, 0x03, 0x00, 0x02, 0x01, 0x03, 0x02, 0x06, 0x03, 0x03, 0x00, 0x82, 0x89, 0x88, 0x8b, 0x87, 0x85, 0x83, 0x85, 0x80, 0x89, 0x89, 0x8b, 0x8c, 0x8e, 0x8b, 0x8b, 0x89, 0x84, 0x84, 0x81, 0x04, 0x06, 0x04, 0x03, 0x03, 0x02, 0x04, 0x03, 0x02, 0x06, 0x05, 0x06, 0x0a, 0x0b, 0x0c, 0x0e, 0x0a, 0x08, 0x06, 0x06, 0x05, 0x07, 0x07, 0x0d, 0x0f, 0x0e, 0x0d, 0x0c, 0x07, 0x04, 0x81, 0x86, 0x89, 0x8b, 0x8d, 0x8a, 0x8a, 0x8c, 0x8b, 0x8d, 0x91, 0x94, 0x95, 0x95, 0x93, 0x93, 0x92, 0x8d, 0x89, 0x86, 0x84, 0x82, 0x83, 0x83, 0x84, 0x84, 0x81, 0x04, 0x0d, 0x16, 0x17, 0x19, 0x17, 0x14, 0x0f, 0x0c, 0x0c, 0x0b, 0x07, 0x07, 0x07, 0x08, 0x09, 0x0d, 0x0c, 0x0e, 0x09, 0x07, 0x80, 0x86, 0x8c, 0x8e, 0x8d, 0x8c, 0x86, 0x86, 0x85, 0x87, 0x87, 0x8b, 0x8b, 0x8f, 0x8e, 0x8e, 0x8c, 0x8a, 0x85, 0x86, 0x80, 0x01, 0x01, 0x00, 0x04, 0x02, 0x05, 0x01, 0x00, 0x00, 0x02, 0x01, 0x08, 0x07, 0x07, 0x03, 0x01, 0x85, 0x87, 0x89, 0x87, 0x81, 0x01, 0x09, 0x0d, 0x10, 0x0f, 0x11, 0x0f, 0x0d, 0x09, 0x08, 0x03, 0x00, 0x84, 0x84, 0x86, 0x84, 0x82, 0x80, 0x85, 0x8a, 0x94, 0x97, 0x9b, 0x99, 0x99, 0x8a, 0x83, 0x06, 0x0c, 0x11, 0x11, 0x12, 0x0e, 0x0c, 0x0d, 0x0c, 0x0d, 0x0c, 0x0a, 0x06, 0x04, 0x01, 0x82, 0x88, 0x90, 0x94, 0x94, 0x97, 0x93, 0x8d, 0x85, 0x00, 0x04, 0x03, 0x08, 0x06, 0x04, 0x01, 0x02, 0x01, 0x05, 0x05, 0x08, 0x0a, 0x0a, 0x06, 0x03, 0x83, 0x87, 0x8c, 0x90, 0x91, 0x92, 0x91, 0x8b, 0x86, 0x80, 0x03, 0x07, 0x09, 0x0a, 0x07, 0x06, 0x0b, 0x0f, 0x0e, 0x0f, 0x11, 0x12, 0x10, 0x0a, 0x04, 0x02, 0x85, 0x88, 0x8c, 0x8e, 0x8f, 0x8c, 0x8a, 0x85, 0x81, 0x01, 0x03, 0x01, 0x84, 0x87, 0x89, 0x8b, 0x8c, 0x89, 0x87, 0x00, 0x04, 0x05, 0x08, 0x0a, 0x08, 0x06, 0x03, 0x83, 0x83, 0x86, 0x83, 0x81, 0x02, 0x01, 0x06, 0x03, 0x02, 0x81, 0x80, 0x84, 0x83, 0x87, 0x85, 0x82, 0x80, 0x00, 0x05, 0x03, 0x04, 0x05, 0x04, 0x03, 0x09, 0x04, 0x07, 0x09, 0x06, 0x06, 0x06, 0x02, 0x01, 0x00, 0x85, 0x85, 0x88, 0x86, 0x86, 0x85, 0x88, 0x83, 0x85, 0x82, 0x80, 0x02, 0x01, 0x02, 0x81, 0x82, 0x84, 0x84, 0x82, 0x02, 0x02, 0x04, 0x05, 0x07, 0x03, 0x04, 0x81, 0x86, 0x8a, 0x8c, 0x8d, 0x88, 0x85, 0x81, 0x04, 0x04, 0x02, 0x03, 0x02, 0x80, 0x80, 0x80, 0x02, 0x05, 0x06, 0x09, 0x0c, 0x0c, 0x0e, 0x0c, 0x08, 0x04, 0x02, 0x82, 0x83, 0x85, 0x86, 0x86, 0x84, 0x87, 0x87, 0x86, 0x86, 0x84, 0x82, 0x83, 0x81, 0x82, 0x84, 0x82, 0x81, 0x00, 0x02, 0x03}
	//anotherThing := []byte{0x04, 0x23, 0x00, 0x97, 0x20, 0x00, 0x18, 0xec, 0x85, 0x40, 0xec, 0x89, 0x42, 0xec, 0x8d, 0x55, 0xec, 0x89, 0x42, 0xec, 0x85, 0x09, 0xf0, 0x81, 0x40, 0xf0, 0x85, 0x42, 0xf0, 0x89, 0x55, 0xf0, 0x85, 0x41, 0xf0, 0x81, 0x40, 0x18}
	// moves the head.
	//hmm := []byte{0x04, 0x04, 0x00, 0x93, 0x10, 0x02, 0xf9}
	// moves the head fast.
	//hmm2 := []byte{0x04, 0x04, 0x00, 0x93, 0x90, 0x00, 0x01}
	//hmm3 := []byte{0x04, 0x0b, 0x00, 0x98, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	//hmm4 := []byte{0x04, 0x05, 0x00, 0x99, 0x12, 0x00, 0xff, 0x7f}
	// hmm5 := []byte{0x04, 0x04, 0x00, 0x94, 0xe7, 0x00, 0x20}
	//hmm6 := []byte{0x04, 0x06, 0x00, 0x10, 0x02, 0x00, 0x00, 0x00, 0x00}
	//hmm7 := []byte{0x04, 0x29, 0x00, 0x04, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00}
	//hmm8 := []byte{0x04, 0x06, 0x00, 0x10, 0x01, 0x00, 0x00, 0x00, 0x00}
	//hmm9 := []byte{0x04, 0x29, 0x00, 0x04, 0xe0, 0x83, 0x00, 0x80, 0x01, 0x0d, 0x07, 0x02, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x01, 0x0d, 0x07, 0x02, 0x04, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x01, 0x0d, 0x07, 0x02, 0x07, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x01, 0x0d, 0x07, 0x02, 0x0a, 0x00}
	//hmm7 := []byte{0x04, 0x06, 0x00, 0x10, 0x02, 0x00, 0x00, 0x00, 0x00}
	//audio , _ := hex.DecodeString("04e9028e02008102830101830384048083058403818307860280820789050186098904038806820002870884000082058481058403820003850787058084068703028707820000840784830481018181078681068503830103860400820386078704008607830100850b88010285098a04068b0a8701058b08808407890a84830686058184078602808103830100810486058182058805018808860580870a850000840b898007850180800487030181008001008305850103840382038183048180008309878309880785810b8e088182088d098185098a098185078301008407870584010688048082078b09008506880882870b870400860b858408880886800685018080058a0782800285068282058608870301860885820987048301058903048200840601890780830487088383058403818104850685000486048401048806818102880b868206870885830b8a048181078803018304860500850483020084078705840102850684810789068280048806008403810382830883830584058381068380028204850583000384020081028405818101820687030285068603038909848105880882830686068481068504830004870482010082038200028607850102850884810585058502048806830002870782830584038181048402008203830281010082038180028306840002820384020284038201018403008202830481810281008100028201800081000080008101808102808001820001820280800181018000008000008000008000800000800080008080008001810000800080008080008000800000800000000000800080000080008000800000800080008080008000800000800080000000000080000000008000800000800000000080008000008000800000000000000000000000000000008000800000800080008080008000800000800080000000008000000000008000008000800080000080008000008000800000800080000080008000000000008000000000800080000080008000808000800080")
	FromUs = FromUs + uint16(1)
	header := buildHeader(0x07, uint16(1))
	hmm12, _ := hex.DecodeString("0408005700007f40430000")
	messageBuffer := &bytes.Buffer{}
	binary.Write(messageBuffer, binary.LittleEndian, header)
	if a <= 5 {
		binary.Write(messageBuffer, binary.LittleEndian, hmm12)
		binary.Write(messageBuffer, binary.LittleEndian, m8f)
		binary.Write(messageBuffer, binary.LittleEndian, fourth)
		//binary.Write(messageBuffer, binary.LittleEndian, hmm2)
		binary.Write(messageBuffer, binary.LittleEndian, audio)
	}
	if a > 5 && a < 8000 {
		binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x02, 0x00, 0x9d, 0xff})
		binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x02, 0x00, 0x9d, 0x01})
		binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x0b, 0x00, 0x40, 0x00, 0x00, 0x20, 0x41, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff})

		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x0b, 0x00, 0x40, 0x00, 0x00, 0x20, 0x41, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff})
		// 00 40 slow, forward.
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x0b, 0x00, 0x40, 0x00, 0x00, 0x00, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0})
		// 543200
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x03, 0x00, 0x54, 0x32, 0x00})
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x03, 0x00, 0x54, 0x32, 0x00})

		// 5700000000000001
		// 5700000040100000
		// 5700007f40100000
		// 5700007f401d0000
		// 5700007f40340000
		// 5700007f40430000
		// 5700007f40430000
		// 5700007f40430000
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x08, 0x00, 0x57, 0x00, 0x00, 0x7f, 0x40, 0x43, 0x00, 0x00})

		// binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x02, 0x00, 0x9b, 0x01})
		// 94e70000
		// lift goes up. veryy slow... (94000080 <-- fast).
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x04, 0x00, 0x94, 0x00, 0x00, 0x80})

		// 990000ff7f
		// forward
		// binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x05, 0x00, 0x99, 0x00, 0x00, 0xff, 0x7f})

		//3914f529be368da7c000002041c2b8b23d00000043
		// moves to a defined position. Maybe the initial pos?.
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x15, 0x00, 0x39, 0xff, 0xf5, 0x29, 0xbe, 0x36, 0x8d, 0xa7, 0xc0, 0x00, 0x00, 0x20, 0x41, 0xc2, 0xb8, 0xb2, 0x3d, 0x00, 0x00, 0x00, 0x43})

		//37 9eae9d3e(POS) 000070410000a0410000000044
		// This is moving the head, to a given position.
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x12, 0x00, 0x37, 0x00, 0x00, 0xff, 0x40, 0x00, 0x00, 0x60, 0xa0, 0xa0, 0x00, 0x30, 0xf0, 0x50, 0xf0, 0x00, 0xff, 0xff})
		//500201
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x03, 0x00, 0x50, 0x02, 0x01})

		// 81002018000004000000000000
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x0d, 0x00, 0x81,0x00, 0x20, 0x18, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		//binary.Write(messageBuffer, binary.LittleEndian, m8f)
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x02, 0x00, 0x9b, 0x01})
		//binary.Write(messageBuffer, binary.LittleEndian, thirdGesicht)
		//binary.Write(messageBuffer, binary.LittleEndian, m8f)
		//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x01, 0x00, 0x9a})
	}
	//if a > 1 {
	//  return
	//  binary.Write(messageBuffer, binary.LittleEndian, fourth)
	//  binary.Write(messageBuffer, binary.LittleEndian, hmm2)
	//  binary.Write(messageBuffer, binary.LittleEndian, hmm6)
	//  binary.Write(messageBuffer, binary.LittleEndian, hmm7)
	//  binary.Write(messageBuffer, binary.LittleEndian, hmm8)
	// binary.Write(messageBuffer, binary.LittleEndian, hmm9)
	// binary.Write(messageBuffer, binary.LittleEndian, m8f)
	//}

	//binary.Write(messageBuffer, binary.LittleEndian, audio)
	a++
	//binary.Write(messageBuffer, binary.LittleEndian, m8f)
	//binary.Write(messageBuffer, binary.LittleEndian, firstGesicht)
	//binary.Write(messageBuffer, binary.LittleEndian, m8f)
	//binary.Write(messageBuffer, binary.LittleEndian, secondGesicht)
	//binary.Write(messageBuffer, binary.LittleEndian, m8f)
	//binary.Write(messageBuffer, binary.LittleEndian, thirdGesicht)
	//binary.Write(messageBuffer, binary.LittleEndian, audio)

	//binary.Write(messageBuffer, binary.LittleEndian, hmm3)

	//binary.Write(messageBuffer, binary.LittleEndian, hmm)
	//binary.Write(messageBuffer, binary.LittleEndian, hmm2)
	//binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x29, 0x00, 0x04, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00, 0xe0, 0x83, 0x00, 0x80, 0x22, 0x64, 0x04, 0x22, 0x00, 0x00})
	toLeftMessage := packet(messageBuffer.Bytes())

	err := binary.Write(buf, binary.LittleEndian, toLeftMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go send(connection, buf, remAddr)
	// fmt.Println(a)
	//fmt.Println("to left")
	//fmt.Println(hex.EncodeToString(buf.Bytes()))

}

func toLeft(connection *net.UDPConn, remAddr *net.UDPAddr) {
	buf := &bytes.Buffer{}
	FromUs = FromUs + uint16(1)
	header := buildHeader(0x07, uint16(1))
	messageBuffer := &bytes.Buffer{}
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x0b, 0x00, 0x33, 0x00, 0x00, 0x10, 0x40, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff})
	toLeftMessage := packet(messageBuffer.Bytes())

	err := binary.Write(buf, binary.LittleEndian, toLeftMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go send(connection, buf, remAddr)
}

func activateStream(connection *net.UDPConn, remAddr *net.UDPAddr) {
	fmt.Println("Activate Stream")
	buf := &bytes.Buffer{}
	// 64ffff
	messageOne := []byte("\x04\x03\x00\x64\xff\xff")
	// 8001000000
	messageTwo := []byte("\x04\x05\x00\x80\x01\x00\x00\x00")
	// 5700000040100000
	messageThree := []byte("\x04\x08\x00\x57\x00\x00\x00\x40\x10\x00\x00")

	FromUs = FromUs + uint16(1)
	header := buildHeader(0x07, uint16(2))

	messageBuffer := &bytes.Buffer{}
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, messageOne)
	binary.Write(messageBuffer, binary.LittleEndian, messageTwo)
	binary.Write(messageBuffer, binary.LittleEndian, messageThree)
	mess := packet(messageBuffer.Bytes())

	err := binary.Write(buf, binary.LittleEndian, mess.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go send(connection, buf, remAddr)
}

func toForward(connection *net.UDPConn, remAddr *net.UDPAddr) {
	buf := &bytes.Buffer{}
	FromUs = FromUs + uint16(1)
	header := buildHeader(0x07, uint16(1))
	messageBuffer := &bytes.Buffer{}
	m8f := []byte{0x04, 0x01, 0x00, 0x8f}
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, m8f)
	binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x05, 0x00, 0x99, 0x00, 0x00, 0xff, 0x7f})
	toLeftMessage := packet(messageBuffer.Bytes())

	err := binary.Write(buf, binary.LittleEndian, toLeftMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go send(connection, buf, remAddr)
}

func liftUp(connection *net.UDPConn, remAddr *net.UDPAddr, up bool) {
	buf := &bytes.Buffer{}
	FromUs = FromUs + uint16(1)
	liftPos.Pos = liftPos.Pos + uint16(5)
	header := buildHeader(0x07, uint16(1))
	messageBuffer := &bytes.Buffer{}
	m8f := []byte{0x04, 0x01, 0x00, 0x8f}
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, m8f)
	if up {
		binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x04, 0x00, 0x94, 0x00, 0x10, 0x80})
	} else {
		binary.Write(messageBuffer, binary.LittleEndian, []byte{0x04, 0x04, 0x00, 0x94, 0x00, 0xf0, 0x20})
	}

	toLeftMessage := packet(messageBuffer.Bytes())

	err := binary.Write(buf, binary.LittleEndian, toLeftMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go send(connection, buf, remAddr)
}

func getch() []byte {
	t, _ := term.Open("/dev/tty")
	term.RawMode(t)
	bytes := make([]byte, 3)
	numRead, err := t.Read(bytes)
	t.Restore()
	t.Close()
	if err != nil {
		return nil
	}
	return bytes[0:numRead]
}

func doOperations(connection *net.UDPConn, remAddr *net.UDPAddr) {
	for {
		//<-time.After(200 * time.Millisecond)
		if connectionReady && resetSend {
			c := getch()
			switch {
			case bytes.Equal(c, []byte{3}):
				return
			case bytes.Equal(c, []byte{27, 91, 68}): // left
				fmt.Println("LEFT pressed")
				go toLeft(connection, remAddr)
			case bytes.Equal(c, []byte{27, 91, 67}): // right
				fmt.Println("RIGHT pressed")
				// go toLeft(connection, remAddr)
			case bytes.Equal(c, []byte{27, 91, 65}): // up
				fmt.Println("UP pressed")
				go liftUp(connection, remAddr, true)
			case bytes.Equal(c, []byte{27, 91, 66}): // down
				fmt.Println("DOWN pressed")
				go liftUp(connection, remAddr, false)
			default:
				// fmt.Println("Unknown pressed", c)
			}
		}
	}
}

func streamVideo(videoStream chan []byte) {
	window := gocv.NewWindow("Hello")
	//options := &webp.DecoderOptions{}
	defer window.Close()
	for range videoStream {
		//imag, err := webp.DecodeRGBA(byt, options)
		//img, err := png.Decode(bytes.NewReader(byt))
		//if err != nil {
		//	panic(err)
		//}
		//bounds := img.Bounds()
		//buf := new(bytes.Buffer)
		//jpeg.Encode(buf, img, nil)
		//image, _ := gocv.NewMatFromBytes(320, 240, gocv.MatTypeCV8UC1, byt)
		//defer image.Close()
		//window.IMShow(image)
		//window.WaitKey(1)
		// f, err := os.OpenFile("/tmp/dat1", os.O_APPEND|os.O_WRONLY, 0600)
		// if err != nil {
		// 	panic(err)
		// }
		// defer f.Close()
		// if _, err = f.Write(byt); err != nil {
		// 	panic(err)
		// }
		// newline := []byte("\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n")
		// if _, err = f.Write(newline); err != nil {
		// 	panic(err)
		// }
	}
}

func listen(connection *net.UDPConn, RemoteAddr *net.UDPAddr) {
	defer connection.Close()
	message := firstpacket()
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.LittleEndian, message)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go send(connection, buf, RemoteAddr)
	go doTick(connection, RemoteAddr)
	go doOperations(connection, RemoteAddr)
	videoStream := make(chan []byte)
	go streamVideo(videoStream)
	for {
		bufInput := make([]byte, 1024)
		amountByte, remAddr, err := connection.ReadFromUDP(bufInput)
		readedMessage := make([]byte, amountByte)
		copy(readedMessage[:], bufInput)
		if err != nil {
			log.Println(err)
		} else {
			//fmt.Println(amountByte, "bytes received from", remAddr)
			reader := bytes.NewReader(readedMessage)
			handleIncomingMessage(reader, connection, remAddr, videoStream)
		}
	}
}

func handleIncomingMessage(reader *bytes.Reader, connection *net.UDPConn, remAddr *net.UDPAddr, videoStream chan []byte) {
	// Check for first bytes.
	if !HasRightFirstBytes(reader) {
		return
	}
	// The next 7 bytes are the Header of the message.
	// Structure is not known yet.
	// We just parse them to a 7 byte array.
	header := Header{}
	headerData := readNextBytes(reader, 7)
	headerBuffer := bytes.NewBuffer(headerData)
	err := binary.Read(headerBuffer, binary.LittleEndian, &header)
	if err != nil {
		log.Fatal("binary.Read Header failed", err)
	}

	// The first byte of the header array should be from Robot.
	var messageType = extractMessageType(header)
	if messageType != messageTypeEnum.FromRobot {
		// We ignore other incoming messages.
		return
	}
	handleIncomingHeaderContent(header)
	// fmt.Println("Start data")
	// Now we do have 3 bytes,
	// Which control the package type.
	// This could be:
	// 0b1100, which after the tock is comes (TockAnswer struct).
	for reader.Len() > 3 {
		typ := readNextBytes(reader, 1)
		length := readNextBytes(reader, 2)
		intlength := int(binary.LittleEndian.Uint16(length))
		if intlength > 0 {
			data := readNextBytes(reader, intlength)
			if typ[0] == 0x0b {
				tock := TockAnswer{}
				buffer := bytes.NewBuffer(data)
				err := binary.Read(buffer, binary.LittleEndian, &tock)
				if err != nil {
					fmt.Println("%d", data)
					fmt.Println("%d", length)
					fmt.Println("%d", typ)
					log.Fatal("binary.Read failed", err)
				}
				handleTick(tock)
			} else if typ[0] == 0x02 {
				// fmt.Println("Type2")
				// Do nothing.
				// Ready for doing ticktock, maybe.
			} else if typ[0] == 0x04 {
				// Do nothing with it.
				// Assumption, next 3 bytes, are the message types.
				// Assumption, ee8ebe stands for json information.
				//fmt.Printf("Data: %d\n", data)
				if data[0] == 0xee {
					// Some defined information is coming.
					dataReader := bytes.NewReader(data)
					// ee8ebe
					fmt.Println("JSON")
					readNextBytes(dataReader, 3)
					len := readNextBytes(dataReader, 2)
					// The next 2 bytes gives us information about the length of the json.
					leng := int(binary.LittleEndian.Uint16(len))
					if leng > 0 {
						readNextBytes(reader, leng)
						// Now, as we have the information,
						// We are ready for ticktock
						readyForTickTock = true
						sendTick(connection, remAddr)
					}
				} else if data[0] == 0xb0 {
					// 0xb0 to be analysed, ignoring for now.
					// fmt.Println("b0...", hex.EncodeToString(data))
				} else if data[0] == 0xc2 {
					// 0xc2 to be analysed, ignoring for now (has no data).
					// fmt.Println("c2...", hex.EncodeToString(data))
				} else if data[0] == 0xd1 {
					// 0xd1 to be analysed.
					// fmt.Println("d1...", hex.EncodeToString(data))
				} else if data[0] == 0xcd {
					// 0xcd to be analysed, looks like it does transfer the movement.
					// fmt.Println("cd... data are you moving??")
				} else if data[0] == 0xcf {
					// fmt.Println("cf...")
				} else if data[0] == 0xc8 {
					// fmt.Println("c8...")
				} else if data[0] == 0xed {
					// 0xed to be analysed, looks like it does say, that it is ready for reset.
					// fmt.Println("ed...")
				} else {
					// typeenco := hex.EncodeToString([]byte{data[0]})
					// fmt.Printf("Type4 type:%s", typeenco)
					// enco := hex.EncodeToString(data)
					// fmt.Println("Type4 data", enco)
				}
			} else if typ[0] == 0x05 {
				// Guessing, f2 is the video stream.
				if data[0] == 0xf2 {
					// fmt.Println("f2...")
					videoReader := bytes.NewReader(data)
					readNextBytes(videoReader, 1)
					readNextBytes(videoReader, 18)
					// Size
					lenv := readNextBytes(videoReader, 2)
					lengv := int(binary.LittleEndian.Uint16(lenv))
					videoStream <- readNextBytes(videoReader, lengv)
				} else if data[0] == 0xf0 {
					// fmt.Println("f0...")
				} else if data[0] == 0xf3 {
					// fmt.Println("f3...")
				} else if data[0] == 0xf1 {
					// fmt.Println("f1...")
					//cameraReady = true
					//if !cameraStreamActivated {
					//  cameraStreamActivated = true
					//  go activateStream(connection, remAddr)
					//}
					// 0xf1 data, is probably telling us, since when Cozmo is awake.
					// As the data part is ascending.
				} else {
					// typeenco := hex.EncodeToString([]byte{data[0]})
					// fmt.Printf("Type5 type:%s", typeenco)
					// enco := hex.EncodeToString(data)
					// fmt.Println("Type5 data", enco)
				}
			} else {
				// fmt.Println("Unknown type: ", typ[0])
			}
		}
	}
	// fmt.Println("End data")
	if connectionReady && !resetSend {
		fmt.Printf("Sending reset\n")
		go send(connection, resetCommand(), remAddr)
		resetSend = true
		cameraReady = true
		if !cameraStreamActivated {
			cameraStreamActivated = true
			go activateStream(connection, remAddr)
		}
	}
	if !connectionReady && tickPack.IteratingReceived[0] > 10 {
		fmt.Printf("Sending connection ready\n")
		go send(connection, getReadyMessage(), remAddr)
		connectionReady = true
	}

}

func HasRightFirstBytes(reader *bytes.Reader) bool {
	// The first 7 bytes should be same as firstBytes().
	firstBy := readNextBytes(reader, 7)
	var firstByteArray [7]byte
	copy(firstByteArray[:], firstBy)
	return (firstByteArray == firstBytes())
}

func extractMessageType(header Header) byte {
	if header.Type == messageTypeEnum.FromRobot {
		return messageTypeEnum.FromRobot
	}
	if header.Type == messageTypeEnum.InitialRequest {
		return messageTypeEnum.InitialRequest
	}
	if header.Type == messageTypeEnum.Tick {
		return messageTypeEnum.Tick
	}
	if header.Type == messageTypeEnum.Unknown7 {
		return messageTypeEnum.Unknown7
	}
	if header.Type == messageTypeEnum.Unknown4 {
		return messageTypeEnum.Unknown4
	}
	return 0x00
}

func handleIncomingHeaderContent(header Header) {
	// We only care for now about "header.FromNext" value.
	// Which we should obviously use as To in the Header
	// for our messages.
	value := binary.LittleEndian.Uint16(header.FromNext[:])
	if value != uint16(0) {
		ToRobotHeader = value
	}
	value = binary.LittleEndian.Uint16(header.To[:])
	if value != uint16(0) {
		FromUs = value
	}
}

func handleTick(tock TockAnswer) {
	if tickMap[string(tock.RandomPart[:])] == 1 {
		delete(tickMap, string(tock.RandomPart[:]))
		// fmt.Println("Tick")
		tickPack.IteratingReceived[0]++
		if tickPack.IteratingReceived[0] == 0 {
			tickPack.IteratingReceived[1]++
			if tickPack.IteratingReceived[1] == 0 {
				tickPack.IteratingReceived[2]++
				if tickPack.IteratingReceived[2] == 0 {
					tickPack.IteratingReceived[3]++
					if tickPack.IteratingReceived[3] == 0 {
						tickPack.IteratingReceived[4]++
					}
				}
			}
		}
	}
}

func readNextBytes(reader *bytes.Reader, number int) []byte {
	bytes := make([]byte, number)

	_, err := reader.Read(bytes)
	if err != nil {
		log.Fatal(err)
	}

	return bytes
}

func send(conn *net.UDPConn, buf *bytes.Buffer, RemoteAddr *net.UDPAddr) {
	conn.WriteToUDP(buf.Bytes(), RemoteAddr)
	//fmt.Println(">>> Request packet sent")
	//    enco := hex.EncodeToString(buf.Bytes())
	//    fmt.Println("%s\n", enco)
	//   if err != nil {
	//       log.Println(err)
	//   }
}
