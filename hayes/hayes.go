package hayes

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tarm/serial"
)

type Cmd string

const (
	ResetCmd         Cmd = "ATZ"
	InfoCmd              = "ATI%d"
	VolumnCmd            = "ATL%d"
	FaxClassesCmd        = "AT+FCLASS=?"
	FaxClassCmd          = "AT+FCLASS?"
	SetFaxClassCmd       = "AT+FCLASS=%s"
	CallerIDModesCmd     = "AT+VCID=?"
	CallerIDCmd          = "AT+VCID?"
	SetCallerIDCmd       = "AT+VCID=%s"
	AnswerCmd            = "ATA"
	HangupCmd            = "ATH0"
)

func FmtCmd(c Cmd, a ...interface{}) Cmd {
	return Cmd(fmt.Sprintf(string(c), a...))
}

type Request struct {
	Cmd      Cmd
	Response chan *Response
}

func NewRequest(c Cmd) *Request {
	return &Request{
		Cmd:      c,
		Response: make(chan *Response),
	}
}

func NewRequestFmt(c Cmd, a ...interface{}) *Request {
	return NewRequest(FmtCmd(c, a...))
}

type Response struct {
	Data string
	Err  error
}

type CallHandler interface {
	Handle(c *Call)
}

type Modem struct {
	tx           chan *Request
	stop         chan struct{}
	stopped      sync.WaitGroup
	cfg          *config
	port         *serial.Port
	portCfg      *serial.Config
	callerIDMode CallerIDMode
	mu           sync.RWMutex
	cache        map[string]interface{}
	cacheEnabled bool
	callHandler  CallHandler
}

func Open(conn string) (*Modem, error) {
	cfg, err := parseConfig(conn)
	if err != nil {
		return nil, err
	}

	portCfg := &serial.Config{
		Name:        cfg.port,
		Baud:        cfg.baud,
		ReadTimeout: 50 * time.Millisecond,
	}

	port, err := serial.OpenPort(portCfg)
	if err != nil {
		return nil, err
	}

	m := &Modem{
		tx:           make(chan *Request),
		stop:         make(chan struct{}),
		cfg:          cfg,
		portCfg:      portCfg,
		port:         port,
		callerIDMode: CallerIDOff,
		cache:        make(map[string]interface{}),
		cacheEnabled: true,
	}

	m.stopped.Add(1)
	go m.run()

	return m, nil
}

func (m *Modem) Close() {
	m.stop <- struct{}{}
	m.stopped.Wait()
}

func (m *Modem) Answer() error {
	rx := make(chan *Response)
	m.tx <- &Request{Cmd: AnswerCmd, Response: rx}
	resp := <-rx
	return resp.Err
}

func (m *Modem) Hangup() error {
	rx := make(chan *Response)
	m.tx <- &Request{Cmd: HangupCmd, Response: rx}
	resp := <-rx
	return resp.Err
}

func (m *Modem) EnableSoftwareCache(v bool) {
	m.cacheEnabled = v
}

func (m *Modem) SetCallHandler(ch CallHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callHandler = ch
}

func (m *Modem) handleCall(c *Call) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callHandler != nil {
		go m.callHandler.Handle(c)
	}
}

func (m *Modem) Reset() error {
	rx := make(chan *Response)
	m.tx <- &Request{Cmd: ResetCmd, Response: rx}
	resp := <-rx
	return resp.Err
}

func (m *Modem) Info() ([]string, error) {
	if v, ok := m.readCache("info"); ok {
		return v.([]string), nil
	}
	infos := []string{}
	for n := 0; n < 10; n++ {
		req := NewRequestFmt(InfoCmd, n)
		m.tx <- req
		resp := <-req.Response
		if resp.Err != nil {
			return nil, resp.Err
		} else if resp.Data == "ERROR" {
			break
		} else if resp.Data == "OK" {
			continue
		}
		infos = append(infos, resp.Data)
	}
	m.writeCache("info", infos)
	return infos, nil
}

type FaxClass string

const (
	FaxClass0   FaxClass = "0"
	FaxClass1            = "1"
	FaxClass1_0          = "1.0"
	FaxClass2            = "2"
	FaxClass8            = "8"
)

// ParseFaxClass parses a string and returns a FaxClass.
func ParseFaxClass(s string) (FaxClass, error) {
	switch s {
	case "0":
		return FaxClass0, nil
	case "1":
		return FaxClass1, nil
	case "1.0":
		return FaxClass1_0, nil
	case "2":
		return FaxClass2, nil
	case "8":
		return FaxClass8, nil
	default:
		return FaxClass0, fmt.Errorf("unrecognized FaxClass: %s", s)
	}
}

// FaxClasses returns the supported fax service classes.
func (m *Modem) FaxClasses() ([]FaxClass, error) {
	if v, ok := m.readCache("faxClasses"); ok {
		return v.([]FaxClass), nil
	}
	req := NewRequest(FaxClassesCmd)
	m.tx <- req
	resp := <-req.Response
	if resp.Err != nil {
		return nil, resp.Err
	}
	a := strings.Split(resp.Data, ",")
	fcs := make([]FaxClass, 0, len(a))
	for _, s := range a {
		fc, err := ParseFaxClass(s)
		if err != nil {
			return nil, err
		}
		fcs = append(fcs, fc)
	}
	m.writeCache("faxClasses", fcs)
	return fcs, nil
}

// FaxClass returns the current fax class.
func (m *Modem) FaxClass() (FaxClass, error) {
	if v, ok := m.readCache("faxClass"); ok {
		return v.(FaxClass), nil
	}
	req := NewRequest(FaxClassCmd)
	m.tx <- req
	resp := <-req.Response
	if resp.Err != nil {
		return FaxClass0, resp.Err
	}
	class, err := ParseFaxClass(resp.Data)
	if err != nil {
		return FaxClass0, err
	}
	m.writeCache("faxClass", class)
	return class, nil
}

// SetFaxClass sets the current fax class.
func (m *Modem) SetFaxClass(fc FaxClass) error {
	if v, ok := m.readCache("faxClass"); ok && v.(FaxClass) == fc {
		return nil
	}
	req := NewRequestFmt(SetFaxClassCmd, fc)
	m.tx <- req
	resp := <-req.Response
	if resp.Err == nil {
		m.writeCache("faxClass", fc)
	}
	return resp.Err
}

type CallerIDMode string

const (
	CallerIDOff         CallerIDMode = "0"
	CallerIDOn                       = "1"
	CallerIDUnformatted              = "2"
)

// ParseCallerIDMode parses a string and returns a CallerIDMode.
func ParseCallerIDMode(s string) (CallerIDMode, error) {
	switch s {
	case "0":
		return CallerIDOff, nil
	case "1":
		return CallerIDOn, nil
	case "2":
		return CallerIDUnformatted, nil
	default:
		return CallerIDOff, fmt.Errorf("unrecognized caller ID mode: %s", s)
	}
}

// CallerIDModes returns the supported caller ID modes.
func (m *Modem) CallerIDModes() ([]CallerIDMode, error) {
	req := NewRequest(CallerIDModesCmd)
	m.tx <- req
	resp := <-req.Response
	if resp.Err != nil {
		return nil, resp.Err
	}

	// Match lower & upper range.  Eg, From "(0-2)", capture the "0" and "2".
	a := regexp.MustCompile("([0-9]*)-([0-9]*)").FindAllStringSubmatch(resp.Data, -1)
	if len(a) != 1 || len(a[0]) != 3 {
		return nil, fmt.Errorf("can't parse(1): %s", resp.Data)
	}
	lower, err := strconv.Atoi(a[0][1])
	if err != nil {
		return nil, fmt.Errorf("can't parse(2): %s", resp.Data)
	}
	upper, err := strconv.Atoi(a[0][2])
	if err != nil {
		return nil, fmt.Errorf("can't parse(3): %s", resp.Data)
	}

	modes := []CallerIDMode{}
	for n := lower; n <= upper; n++ {
		m, err := ParseCallerIDMode(fmt.Sprintf("%d", n))
		if err != nil {
			return nil, err
		}
		modes = append(modes, m)
	}

	return modes, nil
}

// CallerIDMode returns the current caller ID mode.
func (m *Modem) CallerIDMode() (CallerIDMode, error) {
	req := NewRequest(CallerIDCmd)
	m.tx <- req
	resp := <-req.Response
	if resp.Err != nil {
		return CallerIDOff, resp.Err
	}
	var err error
	m.callerIDMode, err = ParseCallerIDMode(resp.Data)

	return m.callerIDMode, err
}

// SetCallerIDMode sets the caller ID mode.
func (m *Modem) SetCallerIDMode(mode CallerIDMode) error {
	req := NewRequestFmt(SetCallerIDCmd, mode)
	m.tx <- req
	resp := <-req.Response
	if resp.Err != nil {
		return resp.Err
	}
	m.callerIDMode = mode
	return nil
}

// SetVolume sets the modem's speaker volume, if it has one.
// n must be 0 - 3
func (m *Modem) SetVolume(n int) error {
	req := NewRequestFmt(VolumnCmd, n)
	m.tx <- req
	resp := <-req.Response
	return resp.Err
}

func (m *Modem) Send(req *Request) error {
	panic("not implemented")
}

func initModem(p *serial.Port) error {
	if _, err := writeRead(p, "AT Z S0=0 E1 V1 Q0"); err != nil {
		return err
	}
	if _, err := writeRead(p, "ATI3"); err != nil {
		return err
	}

	return nil
}

func (m *Modem) run() {
	// Clear modem's buffered responses.
	for {
		r := m.readResponse()
		if r == nil || r.Err != nil {
			break
		}
	}

	var call *Call
	var callSent *Call

	for {
		select {
		case <-m.stop:
			m.stopped.Done()
			return
		case req := <-m.tx:
			// Send command to modem.
			if err := write(m.port, string(req.Cmd)); err != nil {
				req.Response <- &Response{Err: err}
			}
			// Read the echoed command from modem.
			if r := m.readResponse(); r == nil {
				req.Response <- &Response{Err: errors.New("no command echo")}
				continue
			} else if r.Data != string(req.Cmd) {
				req.Response <- &Response{Err: fmt.Errorf("expected %s, got %s", string(req.Cmd), r.Data)}
				continue
			}
			// Read the response from modem.
			r := m.readResponse()
			if r == nil {
				req.Response <- &Response{Err: errors.New("no response")}
				continue
			}
			// Send response to requester.
			req.Response <- r
		default:
		}

		// Read modem initiated events (RINGS, etc.).
		if r := m.readResponse(); r != nil {
			if r.Err != nil {
				// TODO: handle error
				continue
			}
			println(r.Data)
			if r.Data == "RING" {
				if call == nil && callSent == nil {
					call = &Call{Time: time.Now()}
					if m.callerIDMode == CallerIDOff {
						// Not waiting on caller ID so send the call.
						m.handleCall(call)
						callSent = call
						call = nil
						continue
					}
				}
			} else if strings.Contains(r.Data, "NAME = ") {
				if call == nil {
					call = &Call{Time: time.Now()}
				}
				a := strings.Split(r.Data, "=")
				if len(a) == 2 {
					call.Name = strings.TrimSpace(a[1])
					m.handleCall(call)
					callSent = call
					call = nil
				}
			} else if strings.Contains(r.Data, "NMBR = ") {
				if call == nil {
					call = &Call{Time: time.Now()}
				}
				a := strings.Split(r.Data, "=")
				if len(a) == 2 {
					call.Number = strings.TrimSpace(a[1])
				}
			} else if strings.Contains(r.Data, "DATE = ") {
				// ignore it for now
			} else if strings.Contains(r.Data, "TIME = ") {
				// ignore it for now
			}
		} else if r == nil {
			if call != nil && time.Now().Sub(call.Time) > (20*time.Second) {
				// Call was answered, caller hung up early, etc.
				m.handleCall(call)
				call = nil
				callSent = nil
			} else if callSent != nil && time.Now().Sub(callSent.Time) > (20*time.Second) {
				call = nil
				callSent = nil
			}
			continue
		}
	}
}

type Call struct {
	Time   time.Time
	Name   string
	Number string
}

func (m *Modem) readResponse() *Response {
	buf := make([]byte, 0, 1024)
	b := []byte{0}
	for {
		n, err := m.port.Read(b)
		if err != nil && err != io.EOF {
			return &Response{Err: err}
		} else if n == 0 {
			return nil
		}

		switch b[0] {
		case 13: // CR
			if s := string(buf); s != "" {
				return &Response{Data: string(buf)}
			}
			buf = buf[:0]
		case 10: // LF
		default:
			buf = append(buf, b[0])
		}
	}
}

func (m *Modem) readCache(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.cacheEnabled {
		return nil, false
	}
	v, ok := m.cache[key]
	return v, ok
}

func (m *Modem) writeCache(key string, v interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cacheEnabled {
		return
	}
	m.cache[key] = v
}

func writeRead(p *serial.Port, s string) (string, error) {
	if err := write(p, s); err != nil {
		return "", err
	}
	time.Sleep(1000 * time.Millisecond)
	s, err := read(p)
	if err != nil {
		return "", err
	}

	return s, nil
}

func write(p *serial.Port, s string) error {
	cmd := []byte(fmt.Sprintf("%s\r\n", s))
	n, err := p.Write(cmd)
	if err != nil {
		return err
	}

	if n != len(s)+2 {
		return fmt.Errorf("only wrote %d of %d bytes", n, len(s)+2)
	}
	return nil
}

func read(p *serial.Port) (string, error) {
	buf := make([]byte, 1024)
	n, err := p.Read(buf)
	if err != nil {
		return "", err
	}
	s := string(buf[:n])
	fields := strings.Split(s, "\n")
	if len(fields) != 4 {
		return "", fmt.Errorf("invalid response: expected 4 fields, got %d\n", len(fields))
	}
	resp := strings.TrimSpace(fields[2])
	return resp, nil
}

type config struct {
	port string
	baud int
}

func parseConfig(s string) (*config, error) {
	a := strings.Split(s, ",")
	if len(a) != 5 {
		return nil, fmt.Errorf("expected 5 parameters, got %d", len(a))
	}

	baud, err := strconv.Atoi(a[1])
	if err != nil {
		return nil, err
	}

	cfg := &config{
		port: a[0],
		baud: baud,
	}

	return cfg, nil
}
