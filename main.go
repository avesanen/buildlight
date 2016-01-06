package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/tarm/serial"
)

type NeoPixel struct {
	sync.RWMutex
	s      *serial.Port
	leds   int
	In     chan []byte
	Out    chan []byte
	Anim   chan *Anim
	Colors []uint8
}

func New(leds int, port string, baud int) *NeoPixel {
	c := &serial.Config{Name: port, Baud: baud}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}
	np := &NeoPixel{
		leds:   leds,
		s:      s,
		In:     make(chan []uint8, 0),
		Out:    make(chan []uint8, 0),
		Colors: make([]uint8, leds*3),
		Anim:   make(chan *Anim),
	}
	np.Lock()
	go np.reader()
	go np.writer()
	go np.animator()

	return np
}

func (np *NeoPixel) animator() {
	for {
		// Get new animation
		a, ok := <-np.Anim
		if !ok {
			log.Fatal("animator chan closed")
		}

		// Remember previous state
		zero := np.Colors

		// Loop through frames
		for l := a.Loop; l > 0; l-- {
			for _, frame := range a.Frames {
				np.SetColors(frame.Leds)
				np.Sync()
				time.Sleep(frame.Delay * 1000000)
			}
		}

		// Reset leds
		np.SetColors(zero)
		np.Sync()
	}
}

func (np *NeoPixel) Sync() {
	np.Lock()
	np.Out <- np.Colors
}

func (np *NeoPixel) reader() {
	buf := make([]byte, 128)
	for {
		n, err := np.s.Read(buf)
		if err != nil {
			log.Fatal(err.Error())
		}
		log.Printf("Seceived %d bytes: [%s]", n, buf[:n])
		np.Unlock()
		//np.In <- buf:n]
	}
}

func (np *NeoPixel) writer() {
	for {
		b, ok := <-np.Out
		if !ok {
			log.Fatal("writer chan closed")
		}
		n, err := np.s.Write(b)
		if err != nil {
			log.Fatal(err.Error())
		}
		log.Printf("Sent %d bytes", n)
	}
}

func (np *NeoPixel) SetColors(c []uint8) {
	if len(c) != np.leds*3 {
		return
	}

	np.Lock()
	np.Colors = c
	np.Unlock()
}

type Anim struct {
	Loop   int
	Frames []Frame
}

type Frame struct {
	Leds  Leds
	Delay time.Duration
}

type Leds []uint8

func (np *NeoPixel) ColorPOST(w http.ResponseWriter, r *http.Request) {
	var leds Leds
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(body, &leds); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		if err := json.NewEncoder(w).Encode(err); err != nil {
			panic(err)
		}
		log.Println(err.Error())
	}
	np.SetColors(leds)
	np.Sync()
	w.WriteHeader(http.StatusOK)
}

func (np *NeoPixel) AnimPOST(w http.ResponseWriter, r *http.Request) {
	var anim Anim
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(body, &anim); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		if err := json.NewEncoder(w).Encode(err); err != nil {
			panic(err)
		}
		log.Println(err.Error())
	}
	np.Anim <- &anim
	w.WriteHeader(http.StatusOK)
}

func main() {
	np := New(16, "COM7", 115200)

	time.Sleep(time.Second * 1)
	http.HandleFunc("/color", np.ColorPOST)
	http.HandleFunc("/anim", np.AnimPOST)
	http.ListenAndServe(":8080", nil)
}
