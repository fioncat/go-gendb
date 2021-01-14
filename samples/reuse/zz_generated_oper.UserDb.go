// Code generated by go-gendb. DO NOT EDIT.
// go-gendb version: 0.1.2
// source: samples/reuse/user.go
package user

import (
	runner "github.com/fioncat/go-gendb/api/sqlrunner"
	sql "database/sql"
	strings "strings"
)

const (
	sql_UserDb_Adds_0   = "INSERT INTO user(id, name, age, is_admin) VALUES"
	sql_UserDb_Adds_1   = "(?, ?, ?, ?),"
	sql_UserDb_Detail_0 = "SELECT u.id, u.name, u.age, u.is_admin, ud.text, ud.balance, ud.score FROM user u JOIN user_detail ud ON u.id=ud.user_id WHERE"
	sql_UserDb_Detail_1 = "%v=? AND"
	sql_UserDb_FindById = "SELECT id, name, age, is_admin FROM user WHERE id=?"
	sql_UserDb_Search_0 = "SELECT u.id, u.name, u.age, u.is_admin FROM user u WHERE"
	sql_UserDb_Search_1 = "%v%v? AND"
	sql_UserDb_Search_2 = "LIMIT ?, ?"
)

// _UserDbImpl implement of UserDb
type _UserDbImpl struct {
}

var UserDbOper UserDb = &_UserDbImpl{}

// Adds implement of UserDb.Adds
func (*_UserDbImpl) Adds(db *sql.DB, users []*User) (int64, error) {
	// >>> concat start.
	pvs := make([]interface{}, 0, 4*len(users))
	slice := make([]string, 0, 1+len(users))
	// concat: part 0
	slice = append(slice, sql_UserDb_Adds_0)
	// concat: part 1
	lastidx := len(sql_UserDb_Adds_1) - 1
	for i, u := range users {
		pvs = append(pvs, u.Id, u.Name, u.Age, u.IsAdmin)
		if i == len(users) - 1 {
			slice = append(slice, sql_UserDb_Adds_1[:lastidx])
		} else {
			slice = append(slice, sql_UserDb_Adds_1)
		}
	}
	// do concat
	_sql := strings.Join(slice, " ")
	// >>> concat done.
	return runner.ExecAffect(db, _sql, nil, pvs)
}

// Detail implement of UserDb.Detail
func (*_UserDbImpl) Detail(db *sql.DB, conds []Cond) ([]*User, error) {
	// >>> concat start.
	pvs := make([]interface{}, 0, len(conds))
	rvs := make([]interface{}, 0, len(conds))
	slice := make([]string, 0, 1+len(conds))
	// concat: part 0
	slice = append(slice, sql_UserDb_Detail_0)
	// concat: part 1
	lastidx := len(sql_UserDb_Detail_1) - 4
	for i, cond := range conds {
		pvs = append(pvs, cond.Val)
		pvs = append(rvs, cond.Key)
		if i == len(conds) - 1 {
			slice = append(slice, sql_UserDb_Detail_1[:lastidx])
		} else {
			slice = append(slice, sql_UserDb_Detail_1)
		}
	}
	// do concat
	_sql := strings.Join(slice, " ")
	// >>> concat done.
	var os []*User
	err := runner.QueryMany(db, _sql, rvs, pvs, func(rows *sql.Rows) error {
		o := new(User)
		err := rows.Scan(&o.Id, &o.Name, &o.Age, &o.IsAdmin, &o.Text, &o.Balance, &o.Score)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

// FindById implement of UserDb.FindById
func (*_UserDbImpl) FindById(db *sql.DB, id int64) (*User, error) {
	var o *User
	err := runner.QueryOne(db, sql_UserDb_FindById, nil, []interface{}{id}, func(rows *sql.Rows) error {
		o = new(User)
		return rows.Scan(&o.Id, &o.Name, &o.Age, &o.IsAdmin)
	})
	return o, err
}

// Search implement of UserDb.Search
func (*_UserDbImpl) Search(db *sql.DB, conds []Cond, offset int, limit int) ([]*User, error) {
	// >>> concat start.
	pvs := make([]interface{}, 0, 2+len(conds))
	rvs := make([]interface{}, 0, 2*len(conds))
	slice := make([]string, 0, 2+len(conds))
	// concat: part 0
	slice = append(slice, sql_UserDb_Search_0)
	// concat: part 1
	lastidx := len(sql_UserDb_Search_1) - 4
	for i, cond := range conds {
		pvs = append(pvs, cond.Val)
		pvs = append(rvs, cond.Key, cond.Op)
		if i == len(conds) - 1 {
			slice = append(slice, sql_UserDb_Search_1[:lastidx])
		} else {
			slice = append(slice, sql_UserDb_Search_1)
		}
	}
	// concat: part 2
	slice = append(slice, sql_UserDb_Search_2)
	pvs = append(pvs, offset, limit)
	// do concat
	_sql := strings.Join(slice, " ")
	// >>> concat done.
	var os []*User
	err := runner.QueryMany(db, _sql, rvs, pvs, func(rows *sql.Rows) error {
		o := new(User)
		err := rows.Scan(&o.Id, &o.Name, &o.Age, &o.IsAdmin)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}
