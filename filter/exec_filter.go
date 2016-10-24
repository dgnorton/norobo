package filter

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/dgnorton/norobo"
)

type ExecFilter struct {
	cmd  string
	args string
}

func NewExecFilter(cmd, args string) *ExecFilter {
	return &ExecFilter{
		cmd:  cmd,
		args: args,
	}
}

func (e *ExecFilter) Check(c *norobo.Call, result chan *norobo.FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
	go func() {
		defer done.Done()

		var cmdArgs bytes.Buffer

		tmpl, err := template.New("args").Parse(e.args)
		if err != nil {
			panic(err)
		}

		err = tmpl.Execute(&cmdArgs, c)
		if err != nil {
			panic(err)
		}

		fmt.Println(cmdArgs.String())

		// Create command
		cmd := exec.Command(e.cmd, strings.Split(cmdArgs.String(), " ")...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}
		if err := cmd.Start(); err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}

		out := make([]byte, 5)

		defer stdout.Close()
		if _, err := stdout.Read(out); err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}

		done := make(chan error)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
				return
			}
			if string(out[:]) == "block" {
				result <- &norobo.FilterResult{Match: true, Action: e.Action(), Filter: e, Description: "Command returned: block"}
				return
			}
		case <-cancel:
			return
		case <-time.After(10 * time.Second):
			result <- &norobo.FilterResult{Err: errors.New("Exec command timed out"), Action: norobo.Allow, Filter: e, Description: "Exec command timed out"}
			return
		}
		result <- &norobo.FilterResult{Match: false, Action: norobo.Allow, Filter: e, Description: ""}
	}()
}

func (e *ExecFilter) Action() norobo.Action { return norobo.Block }
func (e *ExecFilter) Description() string   { return "Exec command for call return 7 to block" }
