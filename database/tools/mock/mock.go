package mock

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fioncat/go-gendb/compile/parse/pmock"
	"github.com/fioncat/go-gendb/compile/scan/smock"
	"github.com/fioncat/go-gendb/database/rdb"
	"github.com/fioncat/go-gendb/misc/wpool"
)

// Arg is the command line parameter of the mock command.
type Arg struct {
	Exec bool   `flag:"exec"`
	Mute bool   `flag:"mute"`
	File string `flag:"file"`

	Path string `arg:"path"`
}

type sqler struct {
	table     string
	fields    []string
	initField bool
	values    []string
}

func newSqler(name string) *sqler {
	return &sqler{table: name}
}

func (s *sqler) add(fields []*pmock.Field, execArg *pmock.ExecArg) error {
	vals := make([]string, len(fields))
	for idx, f := range fields {
		if !s.initField {
			s.fields = append(s.fields, "`"+f.Name+"`")
		}
		val, err := f.Next(execArg)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s.%s", s.table, f.Name)
		execArg.Vars[key] = fmt.Sprint(val)
		vals[idx] = val2str(val)
	}
	valsStr := strings.Join(vals, ",")
	valsStr = fmt.Sprintf("(%s)", valsStr)
	s.values = append(s.values, valsStr)
	s.initField = true
	return nil
}

func (s *sqler) build() string {
	return fmt.Sprintf("INSERT INTO `%s`(%s) VALUES %s",
		s.table, strings.Join(s.fields, ","),
		strings.Join(s.values, ","))
}

func val2str(v interface{}) string {
	switch v.(type) {
	case float64, int64:
		return fmt.Sprint(v)
	}
	return fmt.Sprintf("'%v'", v)
}

// Do execute mock command.
func Do(arg *Arg) error {
	data, err := ioutil.ReadFile(arg.Path)
	if err != nil {
		return err
	}
	content := string(data)

	sr, err := smock.Do(arg.Path, content)
	if err != nil {
		return err
	}

	result, err := pmock.Do(sr)
	if err != nil {
		return err
	}

	if arg.Exec {
		if err := rdb.MustInit(); err != nil {
			return err
		}
	}

	epochs := make([][]string, result.Epoch)

	var reporter *reporter
	if !arg.Mute {
		initTotal(result.Epoch)
		reporter = newReporter("mocking")
		go reporter.work()
	}

	start := time.Now()
	wp := wpool.New(result.MockWorker, result.Epoch)

	for idx := 0; idx < result.Epoch; idx++ {
		idx := idx
		wp.Submit(func() error {
			return mockWorker(idx, result, epochs)
		})
	}

	err = wp.Wait()
	if !arg.Mute {
		reporter.stop(err)
	}
	if err != nil {
		return err
	}

	if arg.Exec {
		err := exec(arg.Mute, epochs, sr.ExecWorker)
		if err != nil {
			return err
		}
		fmt.Printf("exec done, took: %v\n", time.Since(start))
		return nil
	}

	if arg.File != "" {
		err := writeFile(arg, epochs, sr.Conn)
		if err != nil {
			return err
		}
		fmt.Printf("write file done, tool: %v\n", time.Since(start))
		return nil
	}

	bodys, _ := body(epochs)
	fmt.Println(strings.Join(bodys, "\n"))

	return nil
}

var (
	total    int32
	totalLen string
	current  int32
)

func initTotal(t int) {
	total = int32(t)
	totalLenInt := len(strconv.Itoa(t))
	totalLen = strconv.Itoa(totalLenInt)
	current = 0
}

type reporter struct {
	op         string
	stopCh     chan bool
	stopReport chan struct{}
}

func newReporter(op string) *reporter {
	return &reporter{
		op:         op,
		stopCh:     make(chan bool, 1),
		stopReport: make(chan struct{}, 1),
	}
}

func (r *reporter) work() {
	tk := time.NewTicker(time.Millisecond)
	for {
		select {
		case <-tk.C:
			show(r.op)

		case suc := <-r.stopCh:
			if suc {
				current = total
			}
			show(r.op)
			fmt.Println()
			r.stopReport <- struct{}{}
			return
		}
	}
}

func (r *reporter) stop(err error) {
	r.stopCh <- err == nil
	<-r.stopReport
}

func show(op string) {
	fmt.Printf("%s: %"+totalLen+"d/%d\r", op, current, total)
}

func mockWorker(idx int, result *pmock.Result, epochs [][]string) error {
	sqls, err := epoch(result)
	if err != nil {
		return err
	}
	epochs[idx] = sqls
	atomic.AddInt32(&current, 1)
	return nil
}

func epoch(result *pmock.Result) ([]string, error) {
	execArg := pmock.NewExecArg()
	sqls := make([]string, len(result.Entities))
	for idx, e := range result.Entities {
		num, err := e.Num()
		if err != nil {
			return nil, err
		}
		sqler := newSqler(e.Name)
		var j int64
		for ; j < num; j++ {
			err := sqler.add(e.Fields, execArg)
			if err != nil {
				return nil, err
			}
		}
		sqls[idx] = sqler.build()
	}

	return sqls, nil
}
