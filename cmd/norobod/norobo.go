package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dgnorton/norobo/hayes"
)

func main() {
	var connstr string
	flag.StringVar(&connstr, "c", "/dev/ttyACM0,19200,n,8,1", "serial port connect string (port,baud,handshake,data-bits,stop-bits)")
	flag.Parse()

	modem, err := hayes.Open(connstr)
	check(err)

	modem.SetCallHandler(newCallHandler(modem, "call_log.csv"))
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

	select {}

	modem.Close()
}

type Call struct {
	*hayes.Call
	Spam bool
}

type callHandler struct {
	modem       *hayes.Modem
	block       Filters
	callLogFile string
}

func newCallHandler(m *hayes.Modem, callLogFile string) *callHandler {
	bl, err := LoadListFile("block.csv")
	if err != nil {
		panic(err)
	}

	block := Filters{bl}
	h := &callHandler{modem: m, block: block, callLogFile: callLogFile}
	return h
}

func (h *callHandler) Handle(c *hayes.Call) {
	call := &Call{Call: c}

	result := h.block.MatchAny(call)
	if result.Action == Block {
		if err := h.modem.Answer(); err != nil {
			fmt.Println(err)
		} else if err = h.modem.Hangup(); err != nil {
			fmt.Println(err)
		}
	}

	h.logCall(call, result)
}

func (h *callHandler) logCall(c *Call, r *FilterResult) {
	f, err := os.OpenFile(h.callLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0770)
	if err != nil {
		println(err)
		return
	}
	defer f.Close()
	msg := fmt.Sprintf("%s,%s,%s,%s,%s,%s\n", c.Time, c.Name, c.Number, r.Action, r.FilterDescription(), r.Description)
	if _, err := f.WriteString(msg); err != nil {
		println(err)
	}
	fmt.Printf(msg)
}

func check(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
