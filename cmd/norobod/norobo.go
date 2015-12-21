package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dgnorton/norobo/hayes"
)

func main() {
	var connstr string
	flag.StringVar(&connstr, "c", "/dev/ttyACM0,19200,n,8,1", "serial port connect string (port,baud,handshake,data-bits,stop-bits)")
	flag.Parse()

	modem, err := hayes.Open(connstr)
	check(err)

	modem.SetCallHandler(&callHandler{modem: modem})
	modem.EnableSoftwareCache(false)

	check(modem.Reset())

	infos, err := modem.Info()
	check(err)
	println("Modem info:")
	for _, info := range infos {
		println(info)
	}

	fcs, err := modem.FaxClasses()
	check(err)
	println("Fax classes:")
	for _, fc := range fcs {
		println(fc)
	}

	fc, err := modem.FaxClass()
	check(err)
	fmt.Printf("fax class: %s\n", fc)

	check(modem.SetFaxClass(hayes.FaxClass2))

	fc, err = modem.FaxClass()
	check(err)
	fmt.Printf("fax class: %s\n", fc)

	cidModes, err := modem.CallerIDModes()
	check(err)
	println("Caller ID modes:")
	for _, m := range cidModes {
		println(m)
	}

	cidMode, err := modem.CallerIDMode()
	check(err)
	fmt.Printf("caller ID mode: %s\n", cidMode)

	check(modem.SetCallerIDMode(hayes.CallerIDOn))

	cidMode, err = modem.CallerIDMode()
	check(err)
	fmt.Printf("caller ID mode: %s\n", cidMode)

	time.Sleep(60 * time.Second)

	modem.Close()
}

type callHandler struct {
	modem *hayes.Modem
}

func (h *callHandler) Handle(c *hayes.Call) {
	fmt.Printf("%s [%s] %s\n", c.Time, c.Name, c.Number)
	if c.Name == "Bill Gates" {
		println("spam call")
		if err := h.modem.Answer(); err != nil {
			println(err)
		}
		if err := h.modem.Hangup(); err != nil {
			println(err)
		}
	}
}

func check(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
