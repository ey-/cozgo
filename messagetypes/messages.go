package messagetypes

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"log"
	"os"
)

// Messages struct holds current state of message params.
type Messages struct {
	MessageTypeEnum MessageTypes
	ToRobotHeader   uint16
	FromUs          uint16
}

type Firstpack struct {
	startBytes [7]byte
	Content    [7]byte
}

type TickPackage struct {
	Header            Header
	RandomPart        [7]byte
	Sixtyfour         byte
	IteratingSend     [4]byte
	IteratingReceived [5]byte
}

// TockAnswer Resp type 0b
type TockAnswer struct {
	RandomPart        [7]byte
	Sixtyfour         byte
	IteratingSend     [4]byte
	IteratingReceived [5]byte
}

// Header Resp type 0b
type Header struct {
	Type     byte
	From     [2]byte
	FromNext [2]byte
	To       [2]byte
}

func GenerateRandomPart() [7]byte {
	var randomValue [7]byte
	token := make([]byte, 7)
	rand.Read(token)
	copy(randomValue[:], token)
	return randomValue
}

type MessageTypes struct {
	FromRobot      byte
	InitialRequest byte
	Tick           byte
	Unknown7       byte
	Unknown4       byte
}

func NewMessageTypes() MessageTypes {
	return MessageTypes{
		FromRobot:      0x09,
		InitialRequest: 0x01,
		Tick:           0x0b,
		Unknown7:       0x07,
		Unknown4:       0x04,
	}
}

func FirstBytes() [7]byte {
	var arr [7]byte
	copy(arr[:], []byte("COZ\x03RE\x01"))
	return arr
}

func NewtickPack() TickPackage {
	header := Header{
		Type:     0x0b,
		From:     [2]byte{0, 0},
		FromNext: [2]byte{0, 0},
		To:       [2]byte{0, 0},
	}
	return TickPackage{
		Header:            header,
		RandomPart:        GenerateRandomPart(),
		Sixtyfour:         64,
		IteratingSend:     [4]byte{0, 0, 0, 0},
		IteratingReceived: [5]byte{0, 0, 0, 0, 0},
	}
}

func Firstpacket() Firstpack {
	packet := Firstpack{}
	packet.startBytes = FirstBytes()
	content := []byte("\x01\x01\x00\x01\x00\x00\x00")
	copy(packet.Content[:], content)
	return packet
}

func Packet(Content []byte) *bytes.Buffer {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, FirstBytes())
	binary.Write(buf, binary.LittleEndian, Content)
	return buf
}

func GetMessage(bufInput []byte) []byte {
	// Strip the first 7 bytes
	reader := bytes.NewReader(bufInput)
	message := make([]byte, 2048)
	reader.ReadAt(message, 7)
	return message
}

func (m *Messages) GetReadyMessage() *bytes.Buffer {
	m.FromUs = m.FromUs + uint16(1)
	header := m.BuildHeader(0x04, uint16(0))
	messageBuffer := &bytes.Buffer{}
	// Signal "accepted" or "ready"? Causes robot to return response with 30 length
	// After which we can send the reset command to the robot.
	//readyMessage := []byte("\x00")
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, []byte{0x25})
	ready := Packet(messageBuffer.Bytes())
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.LittleEndian, ready.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	// fmt.Println("%s", hex.EncodeToString(buf.Bytes()))
	return buf
}

func (m *Messages) GetSecondReadyMessage() *bytes.Buffer {
	header := m.BuildHeader(0x04, uint16(0))
	messageBuffer := &bytes.Buffer{}
	readyMessage := []byte("\x64\xff\xff")
	binary.Write(messageBuffer, binary.LittleEndian, header)
	binary.Write(messageBuffer, binary.LittleEndian, readyMessage)
	ready := Packet(messageBuffer.Bytes())
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

func (m *Messages) BuildHeader(typeToUse byte, addFrom uint16) Header {
	header := Header{
		Type:     typeToUse,
		From:     [2]byte{0, 0},
		FromNext: [2]byte{0, 0},
		To:       [2]byte{0, 0},
	}
	toRobot := make([]byte, 2)
	from := make([]byte, 2)
	fromNext := make([]byte, 2)
	binary.LittleEndian.PutUint16(toRobot, m.ToRobotHeader)
	binary.LittleEndian.PutUint16(from, m.FromUs)
	binary.LittleEndian.PutUint16(fromNext, m.FromUs+addFrom)
	m.FromUs = m.FromUs + addFrom
	copy(header.To[:], toRobot)
	copy(header.FromNext[:], fromNext)
	copy(header.From[:], from)
	return header
}

func (m *Messages) ResetCommand() *bytes.Buffer {
	m.FromUs = m.FromUs + uint16(1)
	header := m.BuildHeader(0x07, uint16(0))
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
	binary.Write(messageBuffer, binary.LittleEndian, GenerateRandomBytes(16))

	// 8000000000, effect not visible.
	binary.Write(messageBuffer, binary.LittleEndian, []byte("\x04\x05\x00\x80\x00\x00\x00\x00"))

	resetMessage := Packet(messageBuffer.Bytes())
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

func GenerateRandomBytes(length int) []byte {
	token := make([]byte, length)
	rand.Read(token)
	return token
}

// HasRightFirstBytes checks if the message has the right first bytes.
func HasRightFirstBytes(reader *bytes.Reader) bool {
	// The first 7 bytes should be same as firstBytes().
	firstBy := ReadNextBytes(reader, 7)
	var firstByteArray [7]byte
	copy(firstByteArray[:], firstBy)
	return (firstByteArray == FirstBytes())
}

func (m *Messages) ExtractMessageType(header Header) byte {
	if header.Type == m.MessageTypeEnum.FromRobot {
		return m.MessageTypeEnum.FromRobot
	}
	if header.Type == m.MessageTypeEnum.InitialRequest {
		return m.MessageTypeEnum.InitialRequest
	}
	if header.Type == m.MessageTypeEnum.Tick {
		return m.MessageTypeEnum.Tick
	}
	if header.Type == m.MessageTypeEnum.Unknown7 {
		return m.MessageTypeEnum.Unknown7
	}
	if header.Type == m.MessageTypeEnum.Unknown4 {
		return m.MessageTypeEnum.Unknown4
	}
	return 0x00
}

func (m *Messages) HandleIncomingHeaderContent(header Header) {
	// We only care for now about "header.FromNext" value.
	// Which we should obviously use as To in the Header
	// for our messages.
	value := binary.LittleEndian.Uint16(header.FromNext[:])
	if value != uint16(0) {
		m.ToRobotHeader = value
	}
	value = binary.LittleEndian.Uint16(header.To[:])
	if value != uint16(0) {
		m.FromUs = value
	}
}

func ReadNextBytes(reader *bytes.Reader, number int) []byte {
	bytes := make([]byte, number)

	_, err := reader.Read(bytes)
	if err != nil {
		log.Fatal(err)
	}

	return bytes
}
