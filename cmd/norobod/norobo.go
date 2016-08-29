package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
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

	callHandler := newCallHandler(modem, "call_log.csv")
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

	//select {}

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

	handler.mux.Handle("/", http.FileServer(http.Dir("./web")))
	handler.mux.HandleFunc("/calls", handler.serveCalls)

	return handler
}

func (h *webHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *webHandler) serveCalls(w http.ResponseWriter, r *http.Request) {
	<-h.callHandler.CallLogChanged(time.Now())
	log := h.callHandler.CallLog()
	b, err := json.Marshal(log)
	if err != nil {
		panic(err)
	}
	w.Header().Add("content-type", "application/json")
	w.Write(b)
}

// Call represents the current in-progress call.
type Call struct {
	*hayes.Call
	FilterResult *FilterResult
}

// CallEntry represents a completed (ended) call.
type CallEntry struct {
	Time   time.Time `json:"time"`
	Name   string    `json:"name"`
	Number string    `json:"number"`
	Action string    `json:"action"`
	Filter string    `json:"filter"`
	Rule   string    `json:"rule"`
}

// CallLog represents a list of completed (ended) calls.
type CallLog struct {
	Calls []*CallEntry `json:"calls"`
}

func (l *CallLog) LastTime() time.Time {
	return l.Calls[len(l.Calls)-1].Time
}

func LoadCallLog(filename string) (*CallLog, error) {
	f, err := os.Open(filename)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return &CallLog{
			Calls: make([]*CallEntry, 0),
		}, nil
	}

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}

	calls := &CallLog{
		Calls: make([]*CallEntry, 0, len(records)),
	}

	for _, r := range records {
		if len(r) != 6 {
			return nil, fmt.Errorf("expected 6 fields but got %d: %s", len(r), strings.Join(r, ","))
		}

		t, err := time.Parse(time.RFC3339Nano, r[0])
		if err != nil {
			return nil, err
		}

		call := &CallEntry{
			Time:   t,
			Name:   r[1],
			Number: r[2],
			Action: r[3],
			Filter: r[4],
			Rule:   r[5],
		}

		calls.Calls = append(calls.Calls, call)
	}

	return calls, nil
}

type callHandler struct {
	modem          *hayes.Modem
	block          Filters
	callLogFile    string
	mu             sync.RWMutex
	callLog        *CallLog
	callLogChanged chan struct{}
}

func newCallHandler(m *hayes.Modem, callLogFile string) *callHandler {
	bl, err := LoadFilterFile("block.csv", Block, Allow)
	if err != nil {
		panic(err)
	}

	callLog, err := LoadCallLog(callLogFile)
	if err != nil {
		panic(err)
	}

	block := Filters{bl}

	h := &callHandler{
		modem:          m,
		block:          block,
		callLogFile:    callLogFile,
		callLog:        callLog,
		callLogChanged: make(chan struct{}),
	}

	return h
}

func (h *callHandler) Handle(c *hayes.Call) {
	call := &Call{Call: c}

	call.FilterResult = h.block.Run(call)
	if call.FilterResult.Action == Block {
		call.Block()
	}

	h.log(call)
}

func (h *callHandler) CallLog() *CallLog {
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

func (h *callHandler) log(c *Call) {
	f, err := os.OpenFile(h.callLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0770)
	if err != nil {
		println(err)
		return
	}
	defer f.Close()
	r := c.FilterResult
	msg := fmt.Sprintf(`"%s","%s","%s","%s","%s","%s"%s`, c.Time.Format(time.RFC3339Nano), c.Name, c.Number, r.Action, r.FilterDescription(), r.Description, "\n")

	h.mu.Lock()
	call := &CallEntry{
		Time:   c.Time,
		Name:   c.Name,
		Number: c.Number,
		Action: r.Action.String(),
		Filter: r.FilterDescription(),
		Rule:   r.Description,
	}

	h.callLog.Calls = append(h.callLog.Calls, call)
	close(h.callLogChanged)
	h.callLogChanged = make(chan struct{})
	h.mu.Unlock()

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
