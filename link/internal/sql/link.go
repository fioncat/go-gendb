package sql

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fioncat/go-gendb/coder"
	"github.com/fioncat/go-gendb/compile/golang"
	"github.com/fioncat/go-gendb/compile/sql"
	"github.com/fioncat/go-gendb/database/rdb"
	"github.com/fioncat/go-gendb/link/internal/refs"
	"github.com/fioncat/go-gendb/misc/log"
)

type Linker struct{}

const (
	dbUse   = "db_use"
	runPath = "run_path"
	runName = "run_name"
)

func (*Linker) DefaultConf() map[string]string {
	return map[string]string{
		dbUse:   "db",
		runPath: "github.com/fioncat/go-gendb/api/sql/run",
		runName: "run",
	}
}

func (*Linker) Do(file *golang.File, conf map[string]string) (
	[]coder.Target, error,
) {
	start := time.Now()
	// Each tagged interface generate one target.
	ts := make([]coder.Target, 0, len(file.Interfaces))
	for _, inter := range file.Interfaces {
		if inter.Tag.Name != "sql" {
			continue
		}
		t, err := createTarget(file, inter)
		if err != nil {
			return nil, err
		}
		t.conf = conf
		ts = append(ts, t)
	}

	log.Infof("[link] [sql] [%v] %s, %d target(s)",
		time.Since(start), file.Path, len(ts))

	return ts, nil
}

func createTarget(file *golang.File, inter *golang.Interface) (
	*target, error,
) {
	// import sql method(s)
	sqlm0 := make(map[string]*sql.Method)
	sqlm1 := make(map[string]*sql.Method)

	var sqlPaths []string
	var name string
	for _, opt := range inter.Tag.Options {
		if opt.Value == "" {
			continue
		}
		switch opt.Key {
		case "", "name":
			name = opt.Value

		case "file":
			sqlPaths = append(sqlPaths, opt.Value)
		}
	}
	if name == "" {
		return nil, inter.Tag.FmtError("missing name")
	}
	if len(sqlPaths) == 0 {
		return nil, inter.Tag.FmtError("missing path")
	}

	for _, sqlPath := range sqlPaths {
		v, err := refs.Import(
			file.Path, sqlPath, "sql")
		if err != nil {
			return nil, err
		}
		file := v.(*sql.File)
		for _, m := range file.Methods {
			if m.Inter == name {
				sqlm0[m.Name] = m
				continue
			}
			sqlm1[m.Name] = m
		}
	}
	t := new(target)
	t.file = file
	t.name = name

	t.importMap = make(map[string]*golang.Import)
	for _, imp := range file.Imports {
		var name string
		if imp.Name != "" {
			name = imp.Name
		} else {
			name = filepath.Base(imp.Path)
		}
		t.importMap[name] = imp
	}

	t.methods = make([]*method, len(inter.Methods))
	for idx, goMethod := range inter.Methods {
		sqlMethod := sqlm0[goMethod.Name]
		if sqlMethod == nil {
			sqlMethod = sqlm1[goMethod.Name]
		}
		if sqlMethod == nil {
			return nil, goMethod.FmtError(`can not `+
				`find method "%s" in sql file`, goMethod.Name)
		}
		m := new(method)
		m.sql = sqlMethod
		m.base = goMethod
		if sqlMethod.Exec {
			err := setExecMethodType(goMethod, m)
			if err != nil {
				return nil, err
			}
		} else {
			if goMethod.RetSlice {
				m.Type = queryMulti
			} else {
				m.Type = queryOne
			}
		}

		isAutoRet := false
		for _, tag := range goMethod.Tags {
			if tag.Name == "auto-ret" {
				isAutoRet = true
				break
			}
		}
		if isAutoRet {
			ret, err := autoRet(goMethod, sqlMethod)
			if err != nil {
				return nil, err
			}
			ret.methodName = fmt.Sprintf("%s.%s",
				t.name, goMethod.Name)
			t.rets = append(t.rets, ret)
		}

		t.methods[idx] = m
	}

	return t, nil
}

func setExecMethodType(goMethod *golang.Method, m *method) error {
	var execType int
	switch goMethod.RetType {
	case "sql.Result":
		execType = execResult

	case "int64":
		var lastId bool
		for _, tag := range goMethod.Tags {
			if tag.Name == "lastid" {
				lastId = true
				break
			}
		}
		if lastId {
			execType = execLastId
		} else {
			execType = execAffect
		}

	default:
		return goMethod.FmtError(`Exec sql only `+
			`support returns "sql.Result" or "int64", `+
			`found: "%s"`, goMethod.RetType)
	}

	m.Type = execType
	return nil
}

func autoRet(goMethod *golang.Method, sqlMethod *sql.Method) (
	*ret, error,
) {
	if err := rdb.MustInit(); err != nil {
		return nil, goMethod.FmtError(`auto-ret ` +
			`must set database connection`)
	}
	if sqlMethod.Exec {
		return nil, goMethod.FmtError(`exec sql ` +
			`do not support auto-ret`)
	}
	if goMethod.RetSimple {
		return nil, goMethod.FmtError(`simple type `+
			`"%s" do not support auto-ret`,
			goMethod.RetType)
	}
	retName := goMethod.RetType
	if goMethod.RetSlice {
		retName = strings.TrimPrefix(retName, "[]")
	}
	if goMethod.RetPointer {
		retName = strings.TrimPrefix(retName, "*")
	}
	r := new(ret)
	r.name = retName
	r.fields = make([]*retField, len(sqlMethod.Fields))
	for idx, queryField := range sqlMethod.Fields {
		table, err := rdb.Get().Desc(queryField.Table)
		if err != nil {
			return nil, goMethod.FmtError("desc "+
				"table failed: %v", err)
		}

		dbField := table.Field(queryField.Name)
		if dbField == nil {
			return nil, goMethod.FmtError(`can not `+
				`find field "%s" in table "%s"`,
				queryField.Name, queryField.Table)
		}

		retField := new(retField)
		if queryField.Alias != "" {
			retField.name = queryField.Alias
		}
		if retField.name == "" {
			retField.name = coder.GoName(queryField.Name)
		}
		fType := rdb.Get().GoType(dbField.GetType())
		retField._type = fType
		retField.table = queryField.Table
		retField.field = queryField.Name

		r.fields[idx] = retField
	}
	return r, nil
}
