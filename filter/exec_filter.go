package filter

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"text/template"

	"github.com/dgnorton/norobo"
)

type ExtFilter struct {
	cmd  string
	args string
}

func NewExtFilter(cmd, args string) *ExtFilter {
	return &ExtFilter{
		cmd:  cmd,
		args: args,
	}
}

func (e *ExtFilter) Check(c *norobo.Call, result chan *norobo.FilterResult, cancel chan struct{}, done *sync.WaitGroup) {
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
		out, err := exec.Command(e.cmd, strings.Split(cmdArgs.String(), " ")...).Output()
		if err != nil {
			result <- &norobo.FilterResult{Err: err, Action: norobo.Allow}
			return
		}

		if string(out[:]) == "Block" {
			result <- &norobo.FilterResult{Match: true, Action: e.Action(), Filter: e, Description: "Command returned: Block"}
			return
		}
		result <- &norobo.FilterResult{Match: false, Action: norobo.Allow, Filter: e, Description: ""}
	}()
}

func (e *ExtFilter) Action() norobo.Action { return norobo.Block }
func (e *ExtFilter) Description() string   { return "Exec command for call return 7 to block" }
