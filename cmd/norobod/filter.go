package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

type Filter interface {
	Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup)
	Description() string
}

type Action string

const (
	Allow Action = "allow"
	Block        = "block"
)

type FilterResult struct {
	Err         error
	Match       bool
	Action      Action
	Filter      Filter
	Description string
}

func (r *FilterResult) FilterDescription() string {
	if r.Filter != nil {
		return r.Filter.Description()
	}
	return ""
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
	return &FilterResult{Match: false, Action: Allow}
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

func lookupRuleFunc(name string) (func(*Call) bool, error) {
	switch name {
	case "NameContainsNumber":
		return NameContainsNumber, nil
	case "NumberContainsName":
		return NumberContainsName, nil
	default:
		return nil, fmt.Errorf("unrecognized rule function: %s", name)
	}
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

func LoadListFile(filepath string) (*BlockList, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	bl := NewBlockList()
	r := csv.NewReader(f)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else if len(record) != 4 {
			return nil, fmt.Errorf("expected 4 fields but found %d", len(record))
		}

		var fn func(*Call) bool
		if record[3] != "" {
			fn, err = lookupRuleFunc(record[3])
			if err != nil {
				return nil, err
			}
		}

		bl.Add(record[0], record[1], record[2], fn)
	}

	return bl, nil
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
				case result <- &FilterResult{Match: true, Action: Block, Filter: f, Description: rule.Description}:
					return
				}
			}
		}
		select {
		case <-cancel:
			return
		case result <- &FilterResult{Match: false, Action: Allow, Filter: f}:
			return
		}
	}()
}

func (f *BlockList) Description() string {
	return f.description
}
