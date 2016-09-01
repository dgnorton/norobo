package norobo

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dgnorton/norobo/hayes"
)

// Call represents the current in-progress call.
type Call struct {
	*hayes.Call
	FilterResult *FilterResult
}

// CallEntry represents a completed (ended) call log entry.
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

// LastTime returns the time of the last call in the log.
func (l *CallLog) LastTime() time.Time {
	return l.Calls[len(l.Calls)-1].Time
}

// LoadCallLog loads a call log from file.
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
