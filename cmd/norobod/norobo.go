//go:generate statik -src=web
//go:generate go fmt statik/statik.go
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/dgnorton/norobo"
	"github.com/dgnorton/norobo/filter"
	"github.com/dgnorton/norobo/hayes"
	"github.com/rakyll/statik/fs"

	_ "github.com/dgnorton/norobo/cmd/norobod/statik"
)

func main() {
	var (
		connstr        string
		blockFile      string
		allowFile      string
		callLogFile    string
		twloAccountSID string
		twloToken      string
		execCommand    string
		execArgs       string
	)

	flag.StringVar(&connstr, "c", "/dev/ttyACM0,19200,n,8,1", "serial port connect string (port,baud,handshake,data-bits,stop-bits)")
	flag.StringVar(&blockFile, "block", "", "path to file containing patterns to block")
	flag.StringVar(&allowFile, "allow", "", "path to file containing patterns to allow")
	flag.StringVar(&callLogFile, "call-log", "", "path to call log file")
	flag.StringVar(&twloAccountSID, "twlo-sid", "", "Twilio account SID")
	flag.StringVar(&twloToken, "twlo-token", "", "Twilio token")
	flag.StringVar(&execCommand, "exec", "", "Command gets executed for every call")
	flag.StringVar(&execArgs, "exec-args", "-n {{.Number}}", "Arguments for exec command; uses text/template; availible vars are (Number, Name, Time)")
	flag.Parse()

	modem, err := hayes.Open(connstr)
	check(err)

	callHandler := newCallHandler(modem, blockFile, allowFile, twloAccountSID, twloToken, callLogFile, execCommand, execArgs)
	modem.SetCallHandler(callHandler)
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

	// Start call log web server.
	s := &http.Server{
		Addr:    ":7080",
		Handler: newWebHandler(callHandler),
	}

	check(s.ListenAndServe())

	modem.Close()
}

type webHandler struct {
	mux         *http.ServeMux
	callHandler *callHandler
}

func newWebHandler(h *callHandler) *webHandler {
	handler := &webHandler{
		mux:         http.NewServeMux(),
		callHandler: h,
	}

	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}
	handler.mux.Handle("/", http.FileServer(statikFS))
	handler.mux.HandleFunc("/calls", handler.serveCalls)

	return handler
}

func (h *webHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *webHandler) serveCalls(w http.ResponseWriter, r *http.Request) {
	//<-h.callHandler.CallLogChanged(time.Now())
	log := h.callHandler.CallLog()
	b, err := json.Marshal(log)
	if err != nil {
		panic(err)
	}
	w.Header().Add("content-type", "application/json")
	w.Write(b)
}

type callHandler struct {
	modem          *hayes.Modem
	filters        norobo.Filters
	callLogFile    string
	mu             sync.RWMutex
	callLog        *norobo.CallLog
	callLogChanged chan struct{}
}

func newCallHandler(m *hayes.Modem, blockFile, allowFile, twloAccountSID, twloToken, callLogFile, execCommand, execArgs string) *callHandler {
	filters := norobo.Filters{}

	if blockFile != "" {
		block, err := filter.LoadFilterFile(blockFile, norobo.Block)
		if err != nil {
			panic(err)
		}
		filters = append(filters, block)
	}

	if allowFile != "" {
		allow, err := filter.LoadFilterFile(allowFile, norobo.Allow)
		if err != nil {
			panic(err)
		}
		filters = append(filters, allow)
	}

	if twloAccountSID != "" && twloToken != "" {
		filters = append(filters, filter.NewTwilio(twloAccountSID, twloToken))
	}

	// Adds external cammand exec to filter list if command exists in flags
	if execCommand != "" {
		filters = append(filters, filter.NewExecFilter(execCommand, execArgs))
	}

	callLog, err := norobo.LoadCallLog(callLogFile)
	if err != nil {
		panic(err)
	}

	h := &callHandler{
		modem:          m,
		filters:        filters,
		callLogFile:    callLogFile,
		callLog:        callLog,
		callLogChanged: make(chan struct{}),
	}

	return h
}

func (h *callHandler) Handle(c *hayes.Call) {
	call := &norobo.Call{Call: c}

	call.FilterResult = h.filters.Run(call)
	if call.FilterResult.Action == norobo.Block {
		call.Block()
	}

	h.log(call)
}

func (h *callHandler) CallLog() *norobo.CallLog {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.callLog
}

func (h *callHandler) CallLogChanged(after time.Time) chan struct{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	changedCh := make(chan struct{})
	ch := h.callLogChanged
	go func() {
		for {
			<-ch

			h.mu.RLock()
			changed := h.callLog.LastTime().After(after)

			if changed {
				close(changedCh)
				h.mu.RUnlock()
				return
			}

			ch = h.callLogChanged
			h.mu.RUnlock()
		}
	}()

	return changedCh
}

func (h *callHandler) log(c *norobo.Call) {
	f, err := os.OpenFile(h.callLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0770)
	if err != nil {
		println(err)
		return
	}
	defer f.Close()
	r := c.FilterResult
	w := csv.NewWriter(f)
	msg := []string{c.Time.Format(time.RFC3339Nano), c.Name, c.Number, r.Action.String(), r.FilterDescription(), r.Description}

	h.mu.Lock()
	call := &norobo.CallEntry{
		Time:   c.Time,
		Name:   c.Name,
		Number: c.Number,
		Action: r.Action.String(),
		Filter: r.FilterDescription(),
		Reason: r.Description,
	}

	h.callLog.Calls = append(h.callLog.Calls, call)
	close(h.callLogChanged)
	h.callLogChanged = make(chan struct{})
	h.mu.Unlock()

	if err := w.Write(msg); err != nil {
		println(err)
	}
	w.Flush()
	fmt.Println(call)
}

func check(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
