// Code generated by go-gendb, DO NOT EDIT.
// go-gendb version: 0.3
// source: samples/dynamic-sql/user.go
package user

import (
	run "github.com/fioncat/go-gendb/api/sql/run"
	sql "database/sql"
	strings "strings"
)

const (
	_UserOper_FindByCond0 = "SELECT id, name, email, phone, age, create_time, password, is_admin, is_delete FROM user WHERE 1=1"
	_UserOper_FindByCond1 = "AND id=?"
	_UserOper_FindByCond2 = "AND name=?"
	_UserOper_FindByCond3 = "AND email=?"
	_UserOper_FindByCond4 = "AND phone=?"
	_UserOper_FindByCond5 = "LIMIT ?, ?"
	_UserOper_Adds0       = "INSERT INTO `user`(`name`, `email`, `phone`, `age`) VALUES"
	_UserOper_Adds1       = "(?, ?, ?, ?),"
)

var UserOper = &_UserOper{}

type User struct {
	Id         int64  `table:"user" field:"id"`
	Name       string `table:"user" field:"name"`
	Email      string `table:"user" field:"email"`
	Phone      string `table:"user" field:"phone"`
	Age        int32  `table:"user" field:"age"`
	CreateTime string `table:"user" field:"create_time"`
	Password   string `table:"user" field:"password"`
	IsAdmin    int32  `table:"user" field:"is_admin"`
	IsDelete   int32  `table:"user" field:"is_delete"`
}

type _UserOper struct {}

func (*_UserOper) FindByCond(conds map[string]interface{}, offset, limit int32) ([]*User, error) {
	// [gendb] dynamic start.
	pvs := make([]interface{}, 0, 6)
	slice := make([]string, 0, 6)
	// concat: part 0
	slice = append(slice, _UserOper_FindByCond0)
	// concat: part 1
	if conds["id"] != nil {
		slice = append(slice, _UserOper_FindByCond1)
		pvs = append(pvs, conds["id"])
	}
	// concat: part 2
	if conds["name"] != nil {
		slice = append(slice, _UserOper_FindByCond2)
		pvs = append(pvs, conds["name"])
	}
	// concat: part 3
	if conds["email"] != nil {
		slice = append(slice, _UserOper_FindByCond3)
		pvs = append(pvs, conds["email"])
	}
	// concat: part 4
	if conds["phone"] != nil {
		slice = append(slice, _UserOper_FindByCond4)
		pvs = append(pvs, conds["phone"])
	}
	// concat: part 5
	slice = append(slice, _UserOper_FindByCond5)
	pvs = append(pvs, offset, limit)
	// concat
	_sql := strings.Join(slice, " ")
	// [gendb] dynamic done.
	var os []*User
	err := run.QueryMany(getDb(), _sql, nil, pvs, func(rows *sql.Rows) error {
		o := new(User)
		err := rows.Scan(&o.Id, &o.Name, &o.Email, &o.Phone, &o.Age, &o.CreateTime, &o.Password, &o.IsAdmin, &o.IsDelete)
		if err != nil {
			return err
		}
		os = append(os, o)
		return nil
	})
	return os, err
}

func (*_UserOper) Adds(us []*User) (int64, error) {
	// [gendb] dynamic start.
	pvs := make([]interface{}, 0, 4*len(us))
	slice := make([]string, 0, 1+len(us))
	// concat: part 0
	slice = append(slice, _UserOper_Adds0)
	// concat: part 1
	lastidx := len(_UserOper_Adds1) - 1
	for i, u := range us {
		pvs = append(pvs, u.Name, u.Email, u.Phone, u.Age)
		if i == len(us) - 1 {
			slice = append(slice, _UserOper_Adds1[:lastidx])
		} else {
			slice = append(slice, _UserOper_Adds1)
		}
	}
	// concat
	_sql := strings.Join(slice, " ")
	// [gendb] dynamic done.
	return run.ExecAffect(getDb(), _sql, nil, pvs)
}