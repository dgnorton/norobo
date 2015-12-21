package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
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

type Call struct {
	*hayes.Call
	Spam bool
}

type callHandler struct {
	modem   *hayes.Modem
	filters Filters
}

func newCallHandler(m *hayes.Modem, f Filters) *callHandler {
	h := &callHandler{modem: m, filters: f}
	return h
}

func (h *callHandler) Handle(c *hayes.Call) {
	call := &Call{Call: c}
	filters := Filters{
		&LocalBlackList{
			Names: []string{"Bill Gates"},
		},
	}

	if result := filters.FirstSpam(call); result != nil {
		fmt.Printf("%s %s %s [blocked]\n", c.Time, c.Name, c.Number)
		return
	}

	fmt.Printf("%s %s %s\n", c.Time, c.Name, c.Number)
}

type Filter interface {
	Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup)
}

type FilterResult struct {
	Err  error
	Spam bool
}

type Filters []Filter

func (a Filters) FirstSpam(c *Call) *FilterResult {
	results, cancel, done := a.run(c)
	for i := 0; i < len(a); i++ {
		result := <-results
		if result.Spam {
			close(cancel)
			done.Wait()
			return result
		}
	}
	done.Wait()
	return nil
}

func (a Filters) run(c *Call) (<-chan *FilterResult, chan struct{}, *sync.WaitGroup) {
	results := make(chan *FilterResult)
	cancel := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(len(a))
	for _, filter := range a {
		go filter.Check(c, results, cancel, wg)
	}
	return results, cancel, wg
}

type LocalBlackList struct {
	Names []string
}

func (f *LocalBlackList) Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()
		for _, name := range f.Names {
			if c.Name == name {
				select {
				case <-cancel:
					return
				case result <- &FilterResult{Spam: true}:
					return
				}
			}
		}
		select {
		case <-cancel:
			return
		case result <- &FilterResult{Spam: false}:
			return
		}
	}()
}

func check(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
