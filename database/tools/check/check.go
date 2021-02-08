package check

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fioncat/go-gendb/build"
	"github.com/fioncat/go-gendb/compile/scan/ssql"
	"github.com/fioncat/go-gendb/compile/token"
	"github.com/fioncat/go-gendb/database/rdb"
	"github.com/fioncat/go-gendb/misc/errors"
	"github.com/fioncat/go-gendb/misc/log"
	"github.com/fioncat/go-gendb/misc/set"
	"github.com/fioncat/go-gendb/misc/term"
	"github.com/fioncat/go-gendb/misc/trace"
	"github.com/fioncat/go-gendb/misc/wpool"
)

// Arg stores the command line parameters of check.
type Arg struct {
	Type      string `flag:"type" default:"mysql"`
	Batch     bool   `flag:"batch"`
	ParamPath string `flag:"p"`
	Log       bool   `flag:"log"`
	Filter    string `flag:"filter"`

	Conn string `arg:"conn"`
	Path string `arg:"path"`
}

// sql check task
type checkTask struct {
	path     string
	name     string
	sql      string
	prepares []interface{}
}

// sql check result
type checkResult struct {
	path   string
	name   string
	result rdb.CheckResult
}

// user-pass-in params.
type params map[string]interface{}

// Scan the provided sql file, and construct an inspection
// task based on the parameters passed in by the user.
func buildTasks(path string, ms params, filter *set.Set) ([]*checkTask, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	scanResult, err := ssql.Do(path, string(data))
	if err != nil {
		return nil, err
	}

	var sqls []ssql.Statement
	for _, sql := range scanResult.Statements {
		if filter != nil && filter.Contains(sql.Name) {
			sqls = append(sqls, sql)
			continue
		}

		// No filter, add query statement
		if len(sql.Tokens) == 0 {
			continue
		}
		if token.SQL_SELECT.Match(sql.Tokens[0]) {
			sqls = append(sqls, sql)
		}
	}

	if len(sqls) == 0 {
		return nil, fmt.Errorf("no sql found")
	}

	tasks := make([]*checkTask, len(sqls))
	for idx, sql := range sqls {
		phs, err := ssql.DoPlaceholders(path,
			sql.LineNum, sql.Origin)
		if err != nil {
			return nil, err
		}

		reps := getValues("replace", sql.Name, phs.Replaces, ms)
		sqlStr := fmt.Sprintf(phs.SQL, reps...)

		pres := getValues("prepare", sql.Name, phs.Prepares, ms)

		tasks[idx] = &checkTask{
			path:     sql.Path,
			name:     sql.Name,
			sql:      sqlStr,
			prepares: pres,
		}
	}
	return tasks, nil
}

// Get the specific value from the parameters provided by
// the user. If it is not provided, it will be empty by
// default, but it will warn in the log.
func getValues(t, sqlName string, names []string, ms params) []interface{} {
	vals := make([]interface{}, len(names))
	for idx, name := range names {
		val := ms[name]
		if val == nil {
			log.Errorf("warning: %s param "+
				"is empty for sql %s", t, sqlName)
			val = ""
		}
		vals[idx] = val
	}

	return vals
}

// Do executes SQL statement check based on the passed
// parameters and directly outputs the check result.
func Do(arg *Arg) error {
	if arg.Log {
		log.Init(true, "")
	}

	err := rdb.Init(arg.Conn, arg.Type)
	if err != nil {
		return err
	}

	tt := trace.NewTimer("check")
	defer tt.Trace()

	tt.Start("fetch")
	paths, err := getPaths(arg)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("no file fetched.")
	}

	tt.Start("init-param")
	var ms params
	if arg.ParamPath != "" {
		data, err := ioutil.ReadFile(arg.ParamPath)
		if err != nil {
			return err
		}
		err = json.Unmarshal(data, &ms)
		if err != nil {
			return fmt.Errorf("unmarshal "+
				"params failed: %v", err)
		}
	} else {
		ms = make(params)
	}

	var filter *set.Set
	if arg.Filter != "" {
		names := strings.Split(arg.Filter, ",")
		filter = set.New(names...)
	}

	tt.Start("scan")
	workTasks := make([][]*checkTask, len(paths))
	wp := wpool.New(build.N_WORKERS, len(paths))
	for idx := range paths {
		idx := idx
		wp.Submit(func() error {
			return scanWorker(idx, paths, ms, filter, workTasks)
		})
	}

	if err := wp.Wait(); err != nil {
		return errors.Trace("scan", err)
	}

	tt.Start("flat-task")
	tasks := make([]*checkTask, 0, len(workTasks))
	for _, curTasks := range workTasks {
		tasks = append(tasks, curTasks...)
	}

	if len(tasks) == 0 {
		return errors.New("no task")
	}

	tt.Start("check")
	results := make([]*checkResult, len(tasks))
	wp = wpool.New(build.N_WORKERS, len(tasks))
	for idx := range tasks {
		idx := idx
		wp.Submit(func() error {
			return checkWorker(idx, results, tasks)
		})
	}
	if err := wp.Wait(); err != nil {
		return errors.Trace("check", err)
	}

	tt.Start("group-show")
	groups := make(map[string][]*checkResult)
	for _, r := range results {
		groups[r.path] = append(groups[r.path], r)
	}

	var (
		errCnt  int
		warnCnt int
		okCnt   int
	)
	for _, path := range paths {
		results := groups[path]
		if results == nil {
			continue
		}
		fmt.Printf("file: %s\n", path)
		for _, r := range results {
			fmt.Printf("  sql: %s", r.name)
			if err := r.result.GetErr(); err != nil {
				errCnt += 1
				fmt.Printf("\n    %s\n", term.Red("error: "+err.Error()))
				continue
			}
			if warns := r.result.GetWarns(); len(warns) > 0 {
				warnCnt += 1
				for _, warn := range warns {
					fmt.Printf("\n    %s\n", term.Warn(warn))
				}
			} else {
				okCnt += 1
				fmt.Println(term.Info(" [ok]"))
			}
		}
		fmt.Println()
	}
	fmt.Printf("\nCheck done, %s OK, %s Error, %s Warn\n",
		term.Info(strconv.Itoa(okCnt)),
		term.Red(strconv.Itoa(errCnt)),
		term.Warn(strconv.Itoa(warnCnt)))
	return nil
}

func scanWorker(idx int, paths []string, ms params, f *set.Set, workTasks [][]*checkTask) error {
	path := paths[idx]
	curTasks, err := buildTasks(path, ms, f)
	if err != nil {
		return err
	}
	workTasks[idx] = curTasks
	return nil
}

func checkWorker(idx int, results []*checkResult, tasks []*checkTask) error {
	task := tasks[idx]
	result, err := rdb.Get().Check(task.sql, task.prepares)
	if err != nil {
		return err
	}

	results[idx] = &checkResult{
		path:   task.path,
		name:   task.name,
		result: result,
	}
	return nil
}

func getPaths(arg *Arg) ([]string, error) {
	if !arg.Batch {
		return []string{arg.Path}, nil
	}

	var paths []string
	err := filepath.Walk(arg.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		filename := filepath.Base(path)
		if strings.HasPrefix(filename, ".") {
			return nil
		}

		if strings.HasSuffix(path, ".sql") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}
