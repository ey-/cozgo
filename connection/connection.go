package connection

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/ey-/cozgo/messagetypes"
	"github.com/ey-/cozgo/video"
	"github.com/pkg/term"
)

type Connection struct {
	remoteAddr            *net.UDPAddr
	localAddr             *net.UDPAddr
	conn                  *net.UDPConn
	tickMap               map[string]int
	tickPack              messagetypes.TickPackage
	connectionReady       bool
	readyForTickTock      bool
	resetSend             bool
	cameraStreamActivated bool
	cameraReady           bool
	a                     int
	Messages              messagetypes.Messages
}

// Init initializes the connection variables.
func (c *Connection) Init() {
	c.ensureTickMap()
	c.readyForTickTock = false
	c.connectionReady = false
	c.resetSend = false
	c.cameraStreamActivated = false
	c.cameraReady = false
	c.a = 0
	c.tickPack = messagetypes.NewtickPack()
}

func (c *Connection) ensureTickMap() {
	if c.tickMap == nil {
		c.tickMap = make(map[string]int)
	}
}

// StartListen starts listened the connection.
func (c *Connection) StartListen(serviceAddress string) {
	remoteAddress, _ := net.ResolveUDPAddr("udp", serviceAddress)
	c.remoteAddr = remoteAddress
	localAddr, _ := net.ResolveUDPAddr("udp", ":0")
	c.localAddr = localAddr

	conn, err := net.ListenUDP("udp", c.localAddr)
	c.conn = conn
	// Exit if some error occured
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	c.startLoop()
}

// Send sends the given buffer to the setup connection endpoint.
func (c *Connection) Send(buf *bytes.Buffer) {
	c.conn.WriteToUDP(buf.Bytes(), c.remoteAddr)
	//fmt.Println(">>> Request packet sent")
	// enco := hex.EncodeToString(buf.Bytes())
	// fmt.Println("%s\n", enco)
}

func (c *Connection) startLoop() {
	defer c.conn.Close()
	message := messagetypes.Firstpacket()
	buf := &bytes.Buffer{}
	err := binary.Write(buf, binary.LittleEndian, message)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	go c.Send(buf)
	go c.doTick()
	go doOperations(c)
	video.Init()
	videoStream := video.GetVideoStream()
	go video.StreamVideo()
	for {
		bufInput := make([]byte, 1024)
		amountByte, _, err := c.conn.ReadFromUDP(bufInput)
		readedMessage := make([]byte, amountByte)
		copy(readedMessage[:], bufInput)
		if err != nil {
			log.Println(err)
		} else {
			//fmt.Println(amountByte, "bytes received from", remAddr)
			reader := bytes.NewReader(readedMessage)
			c.handleIncomingMessage(reader, videoStream)
		}
	}
}

func (c *Connection) sendTick() {
	if !c.readyForTickTock {
		fmt.Println("return")
		return
	}
	buf := &bytes.Buffer{}
	c.tickPack.RandomPart = messagetypes.GenerateRandomPart()
	c.tickPack.IteratingSend[0]++
	if c.tickPack.IteratingSend[0] == 0 {
		c.tickPack.IteratingSend[1]++
		if c.tickPack.IteratingSend[1] == 0 {
			c.tickPack.IteratingSend[2]++
			if c.tickPack.IteratingSend[2] == 0 {
				c.tickPack.IteratingSend[3]++
			}
		}
	}

	head := make([]byte, 2)
	binary.LittleEndian.PutUint16(head, c.Messages.ToRobotHeader)
	copy(c.tickPack.Header.To[:], head)

	tickBuf := &bytes.Buffer{}
	binary.Write(tickBuf, binary.LittleEndian, c.tickPack)
	tickMessage := messagetypes.Packet(tickBuf.Bytes())
	err := binary.Write(buf, binary.LittleEndian, tickMessage.Bytes())
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	if c.tickMap == nil {
		c.tickMap = make(map[string]int)
	}
	c.tickMap[string(c.tickPack.RandomPart[:])] = 1
	if err != nil {
	}
	go c.Send(buf)
}

func (c *Connection) doTick() {
	for {
		<-time.After(50 * time.Millisecond)
		go c.sendTick()
	}
}

func (c *Connection) handleTick(tock messagetypes.TockAnswer) {
	if c.tickMap[string(tock.RandomPart[:])] == 1 {
		delete(c.tickMap, string(tock.RandomPart[:]))
		c.tickPack.IteratingReceived[0]++
		if c.tickPack.IteratingReceived[0] == 0 {
			c.tickPack.IteratingReceived[1]++
			if c.tickPack.IteratingReceived[1] == 0 {
				c.tickPack.IteratingReceived[2]++
				if c.tickPack.IteratingReceived[2] == 0 {
					c.tickPack.IteratingReceived[3]++
					if c.tickPack.IteratingReceived[3] == 0 {
						c.tickPack.IteratingReceived[4]++
					}
				}
			}
		}
	}
}

func (c *Connection) handleIncomingMessage(reader *bytes.Reader, videoStream chan []byte) {
	// Check for first bytes.
	if !messagetypes.HasRightFirstBytes(reader) {
		return
	}
	// The next 7 bytes are the Header of the message.
	// Structure is not known yet.
	// We just parse them to a 7 byte array.
	header := messagetypes.Header{}
	headerData := messagetypes.ReadNextBytes(reader, 7)
	headerBuffer := bytes.NewBuffer(headerData)
	err := binary.Read(headerBuffer, binary.LittleEndian, &header)
	if err != nil {
		log.Fatal("binary.Read Header failed", err)
	}

	// The first byte of the header array should be from Robot.
	var messageType = c.Messages.ExtractMessageType(header)
	if messageType != c.Messages.MessageTypeEnum.FromRobot {
		// We ignore other incoming Messages.
		return
	}
	c.Messages.HandleIncomingHeaderContent(header)
	// fmt.Println("Start data")
	// Now we do have 3 bytes,
	// Which control the package type.
	// This could be:
	// 0b1100, which after the tock is comes (messagetypes.TockAnswer struct).
	for reader.Len() > 3 {
		typ := messagetypes.ReadNextBytes(reader, 1)
		length := messagetypes.ReadNextBytes(reader, 2)
		intlength := int(binary.LittleEndian.Uint16(length))
		if intlength > 0 {
			data := messagetypes.ReadNextBytes(reader, intlength)
			if typ[0] == 0x0b {
				tock := messagetypes.TockAnswer{}
				buffer := bytes.NewBuffer(data)
				err := binary.Read(buffer, binary.LittleEndian, &tock)
				if err != nil {
					log.Fatal("binary.Read failed", err)
				}
				c.handleTick(tock)
			} else if typ[0] == 0x02 {
				// fmt.Println("Type2")
				// Do nothing.
				// Ready for doing ticktock, maybe.
			} else if typ[0] == 0x04 {
				// Do nothing with it.
				// Assumption, next 3 bytes, are the message types.
				// Assumption, ee8ebe stands for json information.
				if data[0] == 0xee {
					// Some defined information is coming.
					dataReader := bytes.NewReader(data)
					// ee8ebe
					fmt.Println("JSON")
					messagetypes.ReadNextBytes(dataReader, 3)
					len := messagetypes.ReadNextBytes(dataReader, 2)
					// The next 2 bytes gives us information about the length of the json.
					leng := int(binary.LittleEndian.Uint16(len))
					if leng > 0 {
						messagetypes.ReadNextBytes(dataReader, leng)
						// Now, as we have the information,
						// We are ready for ticktock
						c.readyForTickTock = true
						c.sendTick()
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
					messagetypes.ReadNextBytes(videoReader, 1)
					messagetypes.ReadNextBytes(videoReader, 18)
					// Size
					lenv := messagetypes.ReadNextBytes(videoReader, 2)
					lengv := int(binary.LittleEndian.Uint16(lenv))
					videoStream <- messagetypes.ReadNextBytes(videoReader, lengv)
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
	if c.connectionReady && !c.resetSend {
		fmt.Printf("Sending reset\n")
		go c.Send(c.Messages.ResetCommand())
		c.resetSend = true
		c.cameraReady = true
		if !c.cameraStreamActivated {
			c.cameraStreamActivated = true
			go ActivateStream(c)
		}
	}
	if !c.connectionReady && c.tickPack.IteratingReceived[0] > 10 {
		fmt.Printf("Sending connection ready\n")
		go c.Send(c.Messages.GetReadyMessage())
		c.connectionReady = true
	}

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

func doOperations(c *Connection) {
	for {
		//<-time.After(200 * time.Millisecond)
		if c.connectionReady && c.resetSend {
			ca := getch()
			switch {
			case bytes.Equal(ca, []byte{3}):
				return
			case bytes.Equal(ca, []byte{27, 91, 68}): // left
				fmt.Println("LEFT pressed")
				go DriveWheels(c, -1000, 1000, -1000, 1000)
			case bytes.Equal(ca, []byte{27, 91, 67}): // right
				fmt.Println("RIGHT pressed")
				// go toLeft(connection, remAddr)
				go DriveWheels(c, 1000, -1000, 1000, -1000)
			case bytes.Equal(ca, []byte{27, 91, 65}): // up
				fmt.Println("UP pressed")
				// go ToForward(c)
				go DriveWheels(c, 1000, 1000, 1000, 1000)
			case bytes.Equal(ca, []byte{27, 91, 66}): // down
				fmt.Println("DOWN pressed")
				go DriveWheels(c, -1000, -1000, -1000, -1000)
				// go LiftUp(c, false)
			case bytes.Equal(ca, []byte{119}): // w
				fmt.Println("w pressed")
				go MoveLift(c, 1000)
			case bytes.Equal(ca, []byte{115}): // s
				fmt.Println("s pressed")
				go MoveLift(c, -1000)
			case bytes.Equal(ca, []byte{101}): // e
				fmt.Println("e pressed")
				go MoveHead(c, 0.5)
			case bytes.Equal(ca, []byte{100}): // d
				fmt.Println("d pressed")
				go MoveHead(c, -0.5)
			default:
				fmt.Println("Unknown pressed", ca)
			}
		}
	}
}
