package goutil

import (
	"testing"
)

var users []user
var util *Common

type user struct {
	Id     int     `json:"id"`
	Name   string  `json:"name"`
	Age    int     `json:"age"`
	Remark *string `json:"remark"`
}

func init() {
	u := user{
		Id:   1,
		Name: "zs",
		Age:  100,
	}
	users = append(users, u)
	u = user{
		Id:   2,
		Name: "ls",
		Age:  101,
	}
	users = append(users, u)
	u = user{
		Id:   3,
		Name: "ww",
		Age:  102,
	}
	users = append(users, u)
	util = &Common{}
}

func TestJf(t *testing.T) {
	data := util.Jf(users, NewJfOption().Where("id=1 or name='ls'").Limit(0, 1))
	if len(util.Jq(data, "id").([]interface{})) == 0 {
		t.Fail()
	}
}
