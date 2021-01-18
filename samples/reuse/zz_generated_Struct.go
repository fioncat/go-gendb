// Code generated by go-gendb. DO NOT EDIT.
// go-gendb version: 0.1.4
// source: samples/reuse/user.go
package user

// User is an auto-generated return type for UserDb.FindById
type User struct {
	// Age 用户年龄
	Age int32 `table:"user" field:"age"`
	// Balance 用户余额
	Balance int32 `table:"user_detail" field:"balance"`
	// Id 用户id
	Id int64 `table:"user" field:"id"`
	// IsAdmin 是否是管理员 0-不是  1-是
	IsAdmin int32 `table:"user" field:"is_admin"`
	// Name 用户名
	Name string `table:"user" field:"name"`
	// Score 用户得分
	Score int32 `table:"user_detail" field:"score"`
	// Text 用户详细文本描述
	Text string `table:"user_detail" field:"text"`
}

