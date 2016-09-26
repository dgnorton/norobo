package filter

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

	"github.com/dgnorton/norobo"
)

type Rule struct {
	Description string
	name        *regexp.Regexp
	number      *regexp.Regexp
	fn          func(*norobo.Call) bool
}

func NewRule(description, name, number string, fn func(*norobo.Call) bool) (r *Rule, err error) {
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

func (r *Rule) Match(call *norobo.Call) bool {
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
	description string
	action      norobo.Action
	Rules       []*Rule
}

func NewLocalFilter(description string, action norobo.Action) *LocalFilter {
	return &LocalFilter{
		description: description,
		action:      action,
	}
}

func LoadFilterFile(filepath string, action norobo.Action) (*LocalFilter, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	bl := NewLocalFilter(filepath, action)
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

		var fn func(*norobo.Call) bool
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

func (l *LocalFilter) Add(description, name, number string, fn func(*norobo.Call) bool) error {
	c, err := NewRule(description, name, number, fn)
	if err != nil {
		return err
	}
	l.Rules = append(l.Rules, c)
	return nil
}

func (f *LocalFilter) Check(c *norobo.Call, result chan *norobo.FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()
		for _, rule := range f.Rules {
			if rule.Match(c) {
				select {
				case <-cancel:
					return
				case result <- &norobo.FilterResult{Match: true, Action: f.action, Filter: f, Description: rule.Description}:
					return
				}
			}
		}
		select {
		case <-cancel:
			return
		case result <- &norobo.FilterResult{Match: false, Action: norobo.Allow, Filter: f}:
			return
		}
	}()
}

func (f *LocalFilter) Description() string   { return f.description }
func (f *LocalFilter) Action() norobo.Action { return f.action }

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

// nameContainsNumber returns true if the caller's name contains their phone number.
// Non alpha-numeric chars are stripped from both strings before testing.
func nameContainsNumber(c *norobo.Call) bool {
	name, number := alphas(c.Name), alphas(c.Number)
	return strings.Contains(name, number)
}

// numberContainsName returns true if the caller's phone number sontains their name.
// Non alpha-numeric chars are stripped from both strings before testing.
func numberContainsName(c *norobo.Call) bool {
	name, number := alphas(c.Name), alphas(c.Number)
	return strings.Contains(number, name)
}

// lookupRuleFunc returns a function given the name of the function.
func lookupRuleFunc(name string) (func(*norobo.Call) bool, error) {
	switch name {
	case "NameContainsNumber":
		return nameContainsNumber, nil
	case "NumberContainsName":
		return numberContainsName, nil
	default:
		return nil, fmt.Errorf("unrecognized rule function: %s", name)
	}
}
