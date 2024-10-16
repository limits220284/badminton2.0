package main

import (
	"fmt"
	"testing"
)

func TestGetCookies(t *testing.T) {
	cookie, err := GetCookies("http://ydfwpt.cug.edu.cn/login.html", "1202211216", "060030")
	fmt.Println(cookie, err)
}
