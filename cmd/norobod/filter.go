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
	// Check runs a call through the filter. Results are returned through the
	// result channel. The caller can stop the filter by closing the cancel
	// channel. The filter will signal the done wait group when it has finished.
	Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup)
	// Action returns the action to be taken when a call matches the filter.
	Action() Action
	// Description returns a description of the filter.
	Description() string
}

type Action string

func (a Action) String() string { return string(a) }

func ParseAction(s string) (Action, error) {
	switch s {
	case Allow.String():
		return Allow, nil
	case Block.String():
		return Block, nil
	default:
		return Allow, fmt.Errorf("unrecognized action: %s", s)
	}
}

const (
	Allow = Action("allow")
	Block = Action("block")
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

func (a Filters) Run(call *Call) *FilterResult {
	// Check filters with Allow action first.
	if result := a.RunAction(Allow, call); result.Match {
		return result
	}

	// Check filters with Block action.
	return a.RunAction(Block, call)
}

func (a Filters) RunAction(action Action, call *Call) *FilterResult {
	results, cancel, done := a.run(action, call)
	for {
		select {
		case <-done:
			return &FilterResult{Match: false, Action: Allow}
		case result := <-results:
			if result.Match {
				close(cancel)
				return result
			}
		}
	}
}

func (a Filters) run(action Action, call *Call) (<-chan *FilterResult, chan struct{}, chan struct{}) {
	//func (a Filters) run(action Action, call *Call) (<-chan *FilterResult, int, chan struct{}, *sync.WaitGroup) {
	results := make(chan *FilterResult)
	cancel := make(chan struct{})
	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	for _, filter := range a {
		if filter.Action() != action {
			continue
		}
		wg.Add(1)
		go filter.Check(call, results, cancel, wg)
	}

	// When all filters are finished, signal the caller.
	go func() { wg.Wait(); close(done) }()

	return results, cancel, done
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

type LocalFilter struct {
	description   string
	action        Action
	noMatchAction Action
	Rules         []*Rule
}

func NewLocalFilter(description string, action, noMatchAction Action) *LocalFilter {
	return &LocalFilter{
		description:   description,
		action:        action,
		noMatchAction: noMatchAction,
	}
}

func LoadFilterFile(filepath string, action, noMatchAction Action) (*LocalFilter, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	bl := NewLocalFilter(filepath, action, noMatchAction)
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

func (l *LocalFilter) Add(description, name, number string, fn func(*Call) bool) error {
	c, err := NewRule(description, name, number, fn)
	if err != nil {
		return err
	}
	l.Rules = append(l.Rules, c)
	return nil
}

func (f *LocalFilter) Check(c *Call, result chan *FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()
		for _, rule := range f.Rules {
			if rule.Match(c) {
				select {
				case <-cancel:
					return
				case result <- &FilterResult{Match: true, Action: f.action, Filter: f, Description: rule.Description}:
					return
				}
			}
		}
		select {
		case <-cancel:
			return
		case result <- &FilterResult{Match: false, Action: f.noMatchAction, Filter: f}:
			return
		}
	}()
}

func (f *LocalFilter) Description() string { return f.description }
func (f *LocalFilter) Action() Action      { return f.action }
