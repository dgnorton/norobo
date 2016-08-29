package main

import (
	"testing"
	"time"

	"github.com/dgnorton/norobo/hayes"
)

func Test_Filters(t *testing.T) {
	// Create a block filter.
	bl, err := LoadFilterFile("block.csv", Block, Allow)
	if err != nil {
		t.Fatal(err)
	}
	// Create an allow filter.
	al := NewLocalFilter("allow filter", Allow, Allow)
	if err := al.Add("testing allow filter", "Good Person", "16495551313", nil); err != nil {
		t.Fatal(err)
	}

	filters := &Filters{al, bl}

	testCalls := []*Call{
		newTestCall("Jane Doe", "5556649888", Allow),
		newTestCall("International Scammer", "16495551212", Block),
		newTestCall("Good Person", "16495551212", Allow),
		newTestCall("1112223333", "1112223333", Block),
		newTestCall("111-222-3333", "1112223333", Block),
		newTestCall("1112223333", "111-222-3333", Block),
		newTestCall("1112223333", "1-111-222-3333", Block),
		newTestCall("1-111-222-3333", "1112223333", Block),
		newTestCall("V1112223333", "111-222-3333", Block),
	}

	for _, call := range testCalls {
		if result := filters.Run(call); result == nil {
			t.Fatal("expected valid pointer")
		} else if result.Err != nil {
			t.Fatal(result.Err)
		} else if result.Action != call.FilterResult.Action {
			t.Logf("unexpected action: %s", result.Action)
			t.Logf("caller: name = %s, number = %s", call.Name, call.Number)
			t.Fatalf("filter = %s, rule = %s", result.FilterDescription(), result.Description)
		}
	}
}

func newTestCall(name, number string, action Action) *Call {
	return &Call{
		Call: &hayes.Call{
			Time:   time.Now(),
			Name:   name,
			Number: number,
		},
		FilterResult: &FilterResult{Action: action},
	}
}
