package sql

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fioncat/go-gendb/coder"
	"github.com/fioncat/go-gendb/compile/golang"
	"github.com/fioncat/go-gendb/compile/sql"
)

type target struct {
	c *conf

	file *golang.File

	importMap map[string]*golang.Import

	name string

	methods []*method

	rets []*ret
}

type ret struct {
	name   string
	fields []*retField

	methodName string
}

type retField struct {
	name  string
	_type string

	table string
	field string
}

const (
	execAffect = iota
	execLastId
	execResult

	queryOne
	queryMulti
)

type method struct {
	Type int

	sql  *sql.Method
	base *golang.Method

	constName string
}

func (t *target) Name() string {
	return t.name
}

func (t *target) Path() string {
	return t.file.Path
}

func (t *target) Imports(*coder.Import) {}

func (t *target) Vars(c *coder.Var, ic *coder.Import) {
	group := c.NewGroup()
	group.Add(t.name, "&_", t.name, "{}")
}

func (t *target) Consts(c *coder.Var, ic *coder.Import) {
	group := c.NewGroup()
	group.Comment("all sql statement(s) to use")
	for _, m := range t.methods {
		constName := fmt.Sprintf("_%s_%s",
			t.name, m.base.Name)
		m.constName = constName
		if !m.sql.Dyn {
			group.Add(constName,
				coder.Quote(m.sql.State.Sql))
			continue
		}
		for idx, dp := range m.sql.Dps {
			if dp.ForJoin != "" {
				dp.State.Sql += dp.ForJoin
			}
			group.Add(constName+strconv.Itoa(idx),
				coder.Quote(dp.State.Sql))
		}
	}
}

func (t *target) StructNum() int {
	return len(t.rets) + 1
}

func (t *target) Struct(idx int, c *coder.Struct, ic *coder.Import) {
	if idx >= len(t.rets) {
		c.SetName("_" + t.name)
		return
	}
	ret := t.rets[idx]
	c.Comment("is a struct auto generated by %s",
		ret.methodName)
	c.SetName(ret.name)
	for _, retField := range ret.fields {
		f := c.AddField()
		f.Set(retField.name, retField._type)
		f.AddTag("table", retField.table)
		f.AddTag("field", retField.field)
	}
}

func (t *target) FuncNum() int {
	return len(t.methods)
}

func (t *target) Func(idx int, c *coder.Function, ic *coder.Import) {
	ic.Add(t.c.runName, t.c.runPath)

	m := t.methods[idx]
	for _, impName := range m.base.Imports {
		imp := t.importMap[impName]
		if imp == nil {
			continue
		}
		ic.Add(imp.Name, imp.Path)
	}
	if !m.sql.Exec {
		ic.Add("", "database/sql")
	}

	var rep = "nil"
	var pre = "nil"

	var hasPre bool
	var hasRep bool

	var constName string

	if m.sql.Dyn {
		constName = "_sql"
		for _, dp := range m.sql.Dps {
			if len(dp.State.Prepares) > 0 {
				hasPre = true
			}
			if len(dp.State.Replaces) > 0 {
				hasRep = true
			}
		}
		if hasPre {
			pre = "pvs"
		}
		if hasRep {
			rep = "rvs"
		}
	} else {
		constName = fmt.Sprintf("_%s_%s", t.name, m.base.Name)
		if len(m.sql.State.Replaces) > 0 {
			rep = strings.Join(m.sql.State.Replaces, ", ")
			rep = fmt.Sprintf("[]interface{}{%s}", rep)
		}
		if len(m.sql.State.Prepares) > 0 {
			pre = strings.Join(m.sql.State.Prepares, ", ")
			pre = fmt.Sprintf("[]interface{}{%s}", pre)
		}
	}

	c.Def(m.base.Name, "(*_", t.name, ") ", m.base.Def)
	if m.sql.Dyn {
		t.dyn(c, m, hasPre, hasRep)
		ic.Add("", "strings")
	}
	t.body(c, m, pre, rep, constName)

}

func (t *target) dyn(c *coder.Function, m *method, hasPre, hasRep bool) {
	c.P(0, "// [dynamic] start.")
	preCap, repCap := dynCalcValsCap(m.sql)
	if hasPre {
		c.P(0, "pvs := make([]interface{}, 0, ", preCap, ")")
	}
	if hasRep {
		c.P(0, "rvs := make([]interface{}, 0, ", repCap, ")")
	}
	c.P(0, "slice := make([]string, 0, ", dynCalcSqlsCap(m.sql), ")")
	initLastidx := false
	for idx, dp := range m.sql.Dps {
		name := fmt.Sprintf("_%s_%s%d",
			t.name, m.base.Name, idx)
		c.P(0, "// [dynamic] part ", idx)

		switch dp.Type {
		case sql.DynamicTypeConst:
			c.P(0, "slice = append(slice, ", name, ")")
			dynAppendVals(0, c, dp)

		case sql.DynamicTypeIf:
			c.P(0, "if ", dp.IfCond, " {")
			c.P(1, "slice = append(slice, ", name, ")")
			dynAppendVals(1, c, dp)
			c.P(0, "}")

		case sql.DynamicTypeFor:
			if dp.ForJoin != "" {
				if initLastidx {
					c.P(0, "lastidx = len(", name, ") - ",
						len(dp.ForJoin))
				} else {
					c.P(0, "lastidx := len(", name, ") - ",
						len(dp.ForJoin))
					initLastidx = true
				}
			}
			dynFor(c, dp)
			dynAppendVals(1, c, dp)
			if dp.ForJoin != "" {
				c.P(1, "if i == len(", dp.ForSlice, ") - 1 {")
				c.P(2, "slice = append(slice, ", name, "[:lastidx])")
				c.P(1, "} else {")
				c.P(2, "slice = append(slice, ", name, ")")
				c.P(1, "}")
			} else {
				c.P(1, "slice = append(slice, ", name, ")")
			}
			c.P(0, "}")
		}
	}
	c.P(0, "// [dynamic] joins")
	c.P(0, "_sql := strings.Join(slice, ", "\" \")")
	c.P(0, "// [dynamic] done")
}

func dynFor(c *coder.Function, dp *sql.DynamicPart) {
	hasIdx := dp.ForJoin != ""
	hasEle := dp.ForEle != ""

	switch {
	case hasIdx, hasEle:
		c.P(0, "for i, ", dp.ForEle,
			" := range ", dp.ForSlice, " {")

	case hasIdx, !hasEle:
		c.P(0, "for i := range ", dp.ForSlice, " {")

	case !hasIdx, hasEle:
		c.P(0, "for _, ", dp.ForEle,
			" := range ", dp.ForSlice, " {")

	default:
		c.P(0, "for range ", dp.ForSlice, " {")
	}
}

func dynAppendVals(nTab int, c *coder.Function, dp *sql.DynamicPart) {
	if len(dp.State.Prepares) > 0 {
		c.P(nTab, "pvs = append(pvs, ",
			strings.Join(dp.State.Prepares, ", "), ")")
	}
	if len(dp.State.Replaces) > 0 {
		c.P(nTab, "rvs = append(rvs, ",
			strings.Join(dp.State.Replaces, ", "), ")")
	}
}

func dynCalcValsCap(m *sql.Method) (string, string) {
	var (
		preCnt = 0
		repCnt = 0

		preSlice []string
		repSlice []string
	)
	for _, dp := range m.Dps {
		switch dp.Type {
		case sql.DynamicTypeConst:
			fallthrough
		case sql.DynamicTypeIf:
			preCnt += len(dp.State.Prepares)
			repCnt += len(dp.State.Replaces)

		case sql.DynamicTypeFor:

			var sliceCap string
			if len(dp.State.Prepares) > 0 {
				if len(dp.State.Prepares) > 1 {
					sliceCap = fmt.Sprintf("%d*len(%s)",
						len(dp.State.Prepares), dp.ForSlice)
				} else {
					sliceCap = fmt.Sprintf("len(%s)",
						dp.ForSlice)
				}
				preSlice = append(preSlice, sliceCap)
			}

			if len(dp.State.Replaces) > 0 {
				if len(dp.State.Replaces) > 1 {
					sliceCap = fmt.Sprintf("%d*len(%s)",
						len(dp.State.Replaces), dp.ForSlice)
				} else {
					sliceCap = fmt.Sprintf("len(%s)",
						dp.ForSlice)
				}
				repSlice = append(repSlice, sliceCap)
			}
		}
	}
	return dynCalcCap(preCnt, preSlice),
		dynCalcCap(repCnt, repSlice)
}

func dynCalcSqlsCap(m *sql.Method) string {
	cap := 0
	var slices []string
	for _, dp := range m.Dps {
		switch dp.Type {
		case sql.DynamicTypeIf:
			fallthrough
		case sql.DynamicTypeConst:
			cap += 1
		case sql.DynamicTypeFor:
			slicecap := fmt.Sprintf("len(%s)", dp.ForSlice)
			slices = append(slices, slicecap)
		}
	}
	return dynCalcCap(cap, slices)
}

func dynCalcCap(cap int, extracts []string) string {
	if cap == 0 {
		if len(extracts) == 0 {
			return "0"
		}
		return strings.Join(extracts, "+")
	}
	if len(extracts) == 0 {
		return fmt.Sprint(cap)
	}
	return fmt.Sprintf("%d+%s", cap, strings.Join(extracts, "+"))
}

func (t *target) body(c *coder.Function, m *method, pre, rep, sqlName string) {
	if m.sql.Exec {
		var call string
		switch m.Type {
		case execAffect:
			call = "ExecAffect"

		case execLastId:
			call = "ExecLastId"

		case execResult:
			call = "Exec"
		}
		c.P(0, "return ", t.c.runName, ".", call, "(",
			t.c.dbUse, ", ", sqlName, ", ", rep, ", ", pre, ")")
		return
	}

	retTypeFull := m.base.RetType
	if m.base.RetPointer {
		retTypeFull = "*" + retTypeFull
	}
	if m.base.RetSlice {
		retTypeFull = "[]" + retTypeFull
	}

	fstrs := make([]string, len(m.sql.Fields))
	for idx, f := range m.sql.Fields {
		var name string
		if f.Alias != "" {
			name = f.Alias
		} else {
			name = coder.GoName(f.Name)
		}
		fstrs[idx] = name
	}

	if m.Type == queryOne {
		c.P(0, "var o ", retTypeFull)
		c.P(0, "err := ", t.c.runName, ".QueryOne(", t.c.dbUse,
			", ", sqlName, ", ", rep, ", ", pre,
			", func(rows *sql.Rows) error {")
		if m.base.RetPointer {
			c.P(1, "o = new(", m.base.RetType, ")")
		}
		if m.base.RetSimple {
			c.P(1, "return rows.Scan(&o)")
		} else {
			c.P(1, "return rows.Scan(", assign(fstrs), ")")
		}
		c.P(0, "})")
		c.P(0, "return o, err")
		return
	}

	c.P(0, "var os ", retTypeFull)
	c.P(0, "err := ", t.c.runName, ".QueryMany(", t.c.dbUse,
		", ", sqlName, ", ", rep, ", ", pre,
		", func(rows *sql.Rows) error {")
	if m.base.RetPointer {
		c.P(1, "o := new(", m.base.RetType, ")")
	} else {
		c.P(1, "var o ", m.base.RetType)
	}
	if m.base.RetSimple {
		c.P(1, "err := rows.Scan(&o)")
	} else {
		c.P(1, "err := rows.Scan(", assign(fstrs), ")")
	}

	c.P(1, "if err != nil {")
	c.P(2, "return err")
	c.P(1, "}")
	c.P(1, "os = append(os, o)")
	c.P(1, "return nil")
	c.P(0, "})")
	c.P(0, "return os, err")
}

func assign(rets []string) string {
	ss := make([]string, len(rets))
	for i, ret := range rets {
		ss[i] = fmt.Sprintf("&o.%s", ret)
	}
	return strings.Join(ss, ", ")
}