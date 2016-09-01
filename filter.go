package norobo

import (
	"fmt"
	"sync"
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
