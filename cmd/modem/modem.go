package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/tarm/serial"
)

var port *serial.Port

func main() {
	var connstr string
	flag.StringVar(&connstr, "c", "/dev/ptyp5,19200,n,8,1", "serial port connect string (port,baud,handshake,data-bits,stop-bits)")
	flag.Parse()

	cfg, err := parseConfig(connstr)
	check(err)

	portCfg := &serial.Config{
		Name:        cfg.port,
		Baud:        cfg.baud,
		ReadTimeout: 100 * time.Millisecond,
	}

	port, err = serial.OpenPort(portCfg)
	check(err)
	fmt.Printf("modem port: %s\n", connstr)

	var (
		fclass = "1"
		vcid   = "0"
	)

	r := bufio.NewReader(port)
	go func() {
		for {
			discardWhitespace(r)
			s, err := r.ReadString('\r')
			if err != nil {
				continue
			}
			s = strings.TrimSpace(s)
			fmt.Printf("-> %s\n", s)
			port.Write([]byte(s))
			port.Write([]byte("\r\n"))
			if s == "ATI3" {
				write(port, "CX93001-EIS_V0.2002-V92\r\n")
			} else if s == "AT+FCLASS=?" {
				write(port, "1,2,1.0,8\r\n")
			} else if s == "AT+FCLASS?" {
				write(port, fmt.Sprintf("%s\r\n", fclass))
			} else if strings.Contains(s, "AT+FCLASS=") {
				fclass = s[10:]
				write(port, "OK\r\n")
			} else if s == "AT+VCID=?" {
				write(port, "(0-2)\r\n")
			} else if s == "AT+VCID?" {
				write(port, fmt.Sprintf("%s\r\n", vcid))
			} else if strings.Contains(s, "AT+VCID=") {
				vcid = s[8:]
				write(port, "OK\r\n")
			} else {
				write(port, "OK\r\n")
			}
		}
	}()

	fmt.Printf("http server on :8087\n")
	http.HandleFunc("/call", serveCall)
	http.ListenAndServe(":8087", nil)
}

func serveCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "/call only accepts POST", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	name := q.Get("name")
	number := q.Get("number")

	fmt.Printf("call from %s at %s\n", name, number)

	write(port, "RING\r\n")
	time.Sleep(500 * time.Millisecond)
	write(port, fmt.Sprintf("NAME = %s\r\n", name))
	write(port, fmt.Sprintf("NUMBER = %s\r\n", number))
	write(port, "RING\r\n")
	time.Sleep(500 * time.Millisecond)
	write(port, "RING\r\n")
	time.Sleep(500 * time.Millisecond)
}

func httpError(err error, w http.ResponseWriter, status int) {
	if err != nil {
		fmt.Println(err)
	}
	http.Error(w, "", status)
}

func write(p *serial.Port, s string) {
	if _, err := p.Write([]byte(s)); err != nil {
		println(err)
		return
	}
	fmt.Printf("<- %s\n", strings.TrimSpace(s))
}

func discardWhitespace(r *bufio.Reader) {
	c, _, err := r.ReadRune()
	if err != nil || !unicode.IsSpace(c) {
		r.UnreadRune()
		r.UnreadRune()
	}
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

func check(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
