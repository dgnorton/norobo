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

		// Create command
		cmd := exec.Command(e.cmd, strings.Split(cmdArgs.String(), " ")...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}
		fmt.Printf("running exec filter: %s %s\n", e.cmd, cmdArgs.String())
		if err := cmd.Start(); err != nil {
			println(err.Error())
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
			fmt.Printf("exec filter returned: %s\n", string(out[:]))
			if string(out[:]) == "block" {
				result <- &norobo.FilterResult{Match: true, Action: e.Action(), Filter: e, Description: "command returned: block"}
				return
			}
		case <-cancel:
			println("exec filter canceled")
			return
		case <-time.After(10 * time.Second):
			println("exec filter timed out")
			result <- &norobo.FilterResult{Err: errors.New("exec command timed out"), Action: norobo.Allow, Filter: e, Description: "exec command timed out"}
			return
		}
		fmt.Printf("exec filter returned: %s\n", string(out[:]))
		result <- &norobo.FilterResult{Match: false, Action: norobo.Allow, Filter: e, Description: ""}
	}()
}

func (e *ExecFilter) Action() norobo.Action { return norobo.Block }
func (e *ExecFilter) Description() string {
	return fmt.Sprintf("%s %s", e.cmd, e.args)
}
