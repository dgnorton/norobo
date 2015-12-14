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

type Modem struct {
	tx      chan *Request
	stop    chan struct{}
	stopped sync.WaitGroup
	cfg     *config
	port    *serial.Port
	portCfg *serial.Config
}

func Open(conn string) (*Modem, error) {
	cfg, err := parseConfig(conn)
	if err != nil {
		return nil, err
	}

	portCfg := &serial.Config{
		Name:        cfg.port,
		Baud:        cfg.baud,
		ReadTimeout: 100 * time.Millisecond,
	}

	port, err := serial.OpenPort(portCfg)
	if err != nil {
		return nil, err
	}

	m := &Modem{
		tx:      make(chan *Request),
		stop:    make(chan struct{}),
		cfg:     cfg,
		portCfg: portCfg,
		port:    port,
	}

	m.stopped.Add(1)
	go m.run()

	return m, nil
}

func (m *Modem) Close() {
	m.stop <- struct{}{}
	m.stopped.Wait()
}

func (m *Modem) Reset() error {
	rx := make(chan *Response)
	m.tx <- &Request{Cmd: ResetCmd, Response: rx}
	resp := <-rx
	return resp.Err
}

func (m *Modem) Info() ([]string, error) {
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
	return fcs, nil
}

// FaxClass returns the current fax class.
func (m *Modem) FaxClass() (FaxClass, error) {
	req := NewRequest(FaxClassCmd)
	m.tx <- req
	resp := <-req.Response
	if resp.Err != nil {
		return FaxClass0, resp.Err
	}
	return ParseFaxClass(resp.Data)
}

// SetFaxClass sets the current fax class.
func (m *Modem) SetFaxClass(fc FaxClass) error {
	req := NewRequestFmt(SetFaxClassCmd, fc)
	m.tx <- req
	resp := <-req.Response
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
	return ParseCallerIDMode(resp.Data)
}

// SetCallerIDMode sets the caller ID mode.
func (m *Modem) SetCallerIDMode(mode CallerIDMode) error {
	req := NewRequestFmt(SetCallerIDCmd, mode)
	m.tx <- req
	resp := <-req.Response
	return resp.Err
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
			//println(r.Data)
		}
	}
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
