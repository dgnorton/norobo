package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/dgnorton/norobo/hayes"
)

func main() {
	var connstr string
	flag.StringVar(&connstr, "c", "/dev/ttyACM0,19200,n,8,1", "serial port connect string (port,baud,handshake,data-bits,stop-bits)")
	flag.Parse()

	modem, err := hayes.Open(connstr)
	check(err)

	modem.SetCallHandler(newCallHandler(modem))
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
	modem *hayes.Modem
	block Filters
}

func newCallHandler(m *hayes.Modem) *callHandler {
	bl := NewBlockList()
	bl.Add("unassigned and used for spoofing", `1?999.*`, `1?999.*`, nil)
	bl.Add("international call scam", `1?876.*`, `1?876.*`, nil)
	bl.Add("international call scam", `1?809.*`, `1?809.*`, nil)
	bl.Add("international call scam", `1?649.*`, `1?649.*`, nil)
	bl.Add("international call scam", `1?284.*`, `1?284.*`, nil)
	bl.Add("charity", `^HOPE$`, "", nil)
	bl.Add("spam", `^V[0-9]*$`, "", nil)
	bl.Add("name unavailable", `.*[Uu]navail.*`, "", nil)
	bl.Add("out-of-area", `.*OUT-OF-AREA.*`, "", nil)
	bl.Add("telemarketer", `.*ELITE WATER.*`, "", nil)
	bl.Add("spam", `.*800 [Ss]ervice.*`, "8554776313", nil)
	bl.Add("name contains number", "", "", NameContainsNumber)
	bl.Add("number contains name", "", "", NumberContainsName)

	block := Filters{bl}
	h := &callHandler{modem: m, block: block}
	return h
}

func (h *callHandler) Handle(c *hayes.Call) {
	call := &Call{Call: c}

	if result := h.block.MatchAny(call); result != nil {
		if err := h.modem.Answer(); err != nil {
			fmt.Println(err)
		} else if err = h.modem.Hangup(); err != nil {
			fmt.Println(err)
		}
		fmt.Printf("%s,%s,%s: blocked,filter=%s,rule=%s\n", c.Time, c.Name, c.Number, result.Filter.Description(), result.Description)
		return
	}

	fmt.Printf("%s %s %s\n", c.Time, c.Name, c.Number)
}

type Filter interface {
	Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup)
	Description() string
}

type FilterResult struct {
	Err         error
	Match       bool
	Filter      Filter
	Description string
}

type Filters []Filter

func (a Filters) MatchAny(c *Call) *FilterResult {
	results, cancel, done := a.run(c)
	for i := 0; i < len(a); i++ {
		result := <-results
		if result.Match {
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

// alphas returns a new string containing only the alpha-numeric characters.
func alphas(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func NameContainsNumber(c *Call) bool {
	name, number := alphas(c.Name), alphas(c.Number)
	return strings.Contains(name, number)
}

func NumberContainsName(c *Call) bool {
	name, number := alphas(c.Name), alphas(c.Number)
	println(name)
	println(number)
	return strings.Contains(number, name)
}

type Rule struct {
	Description string
	name        *regexp.Regexp
	number      *regexp.Regexp
	fn          func(*Call) bool
}

func NewRule(description, name, number string, fn func(*Call) bool) (r *Rule, err error) {
	r = &Rule{}
	r.Description = description
	if name != "" {
		if r.name, err = regexp.Compile(name); err != nil {
			return
		}
	}
	if number != "" {
		r.number, err = regexp.Compile(number)
	}
	r.fn = fn
	return
}

func (r *Rule) Match(call *Call) bool {
	if r.name != nil && r.name.MatchString(call.Name) ||
		r.number != nil && r.number.MatchString(call.Number) {
		return true
	}
	if r.fn != nil {
		return r.fn(call)
	}
	return false
}

type BlockList struct {
	description string
	Rules       []*Rule
}

func NewBlockList() *BlockList {
	return &BlockList{
		description: "local block rules",
	}
}

func (l *BlockList) Add(description, name, number string, fn func(*Call) bool) error {
	c, err := NewRule(description, name, number, fn)
	if err != nil {
		return err
	}
	l.Rules = append(l.Rules, c)
	return nil
}

func (f *BlockList) Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()
		for _, rule := range f.Rules {
			if rule.Match(c) {
				select {
				case <-cancel:
					return
				case result <- &FilterResult{Match: true, Filter: f, Description: rule.Description}:
					return
				}
			}
		}
		select {
		case <-cancel:
			return
		case result <- &FilterResult{Match: false, Filter: f}:
			return
		}
	}()
}

func (f *BlockList) Description() string {
	return f.description
}

func check(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
