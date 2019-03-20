package video

import "gocv.io/x/gocv"

var videoStream chan []byte

// GetVideoStream returns the videoStream channel.
func GetVideoStream() chan []byte {
	return videoStream
}

// Init initializes the videoStream.
func Init() {
	videoStream = make(chan []byte)
}

// StreamVideo starts the video stream.
func StreamVideo() {
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
