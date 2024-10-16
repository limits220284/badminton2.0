package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

var BASIC_HEADERS = map[string]string{
	"Accept":       "*/*",
	"Host":         "ydfwpt.cug.edu.cn",
	"Origin":       "http://ydfwpt.cug.edu.cn",
	"Content-Type": "application/x-www-form-urlencoded",
	"Referer":      "http://ydfwpt.cug.edu.cn/",
	"Connection":   "keep-alive",
}

type Config struct {
	APIs              APIsConfig     `yaml:"apis"`
	User              UserConfig     `yaml:"user"`
	EarliestOrderTime string         `yaml:"earliestOrderTime"`
	Target            []TargetConfig `yaml:"target"`
}

type APIsConfig struct {
	Index      string `yaml:"index"`
	Login      string `yaml:"login"`
	FindOkArea string `yaml:"findOkArea"`
	Order      string `yaml:"order"`
	Pay        string `yaml:"pay"`
}

type UserConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	QmsgKey  string `yaml:"QmsgKey"`
}

type TargetConfig struct {
	Time   int `yaml:"time"`
	Number int `yaml:"number"`
}

type Stock struct {
	TimeNo string `json:"time_no"`
}
type Area struct {
	SName   string `json:"sname"`
	Stock   Stock  `json:"stock"`
	StockID int    `json:"stock_id"`
	ID      int    `json:"id"`
}

type AreaInfo struct {
	ID      int    `json:"id"`
	SName   string `json:"sname"`
	StockID int    `json:"stock_id"`
}

// 读取YAML配置
func getYAMLConfig(filename string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	return config, err
}

func findOkArea(baseurl string, date string) (map[string]AreaInfo, error) {
	urlParsed, err := url.Parse(baseurl)
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"s_date":    date,
		"serviceid": "1",
	}
	query := urlParsed.Query()
	for key, value := range params {
		query.Add(key, value)
	}
	urlParsed.RawQuery = query.Encode()

	client := &http.Client{}
	req, err := http.NewRequest("GET", urlParsed.String(), nil)
	if err != nil {
		return nil, err
	}
	for key, value := range BASIC_HEADERS {
		req.Header.Add(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Object []Area `json:"object"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	log.Println("result:", result)
	areaMap := make(map[string]AreaInfo)
	for _, area := range result.Object {
		timeNo := area.Stock.TimeNo
		t := AreaInfo{
			ID:      area.ID,
			SName:   area.SName,
			StockID: area.StockID,
		}
		areaMap[timeNo+area.SName] = t
	}

	return areaMap, nil
}

func GetCookies(urlStr string, username string, password string) (map[string]string, error) {
	payload := url.Values{
		"dlm":         {username},
		"mm":          {password},
		"yzm":         {"1"},
		"logintype":   {"sno"},
		"continueurl": {""},
		"openid":      {""},
	}

	retries := 20 // 设置最大尝试次数

	for attempt := 1; attempt <= retries; attempt++ {
		// 创建请求
		req, err := http.NewRequest("POST", urlStr, strings.NewReader(payload.Encode()))
		if err != nil {
			log.Printf("创建请求失败: %v\n", err)
			continue
		}

		// 设置请求头
		for key, value := range BASIC_HEADERS {
			req.Header.Set(key, value)
		}

		// 禁止重定向
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		// 发送请求
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("第 %d 次登录请求失败: %v\n", attempt, err)
			if attempt < retries {
				time.Sleep(2 * time.Second) // 等待2秒后重试
				continue
			} else {
				log.Println("已达到最大重试次数，登录失败。")
				return nil, err
			}
		}
		defer resp.Body.Close()

		// 处理响应状态
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("第 %d 次登录请求失败: %s\n", attempt, string(body))
			if attempt < retries {
				time.Sleep(2 * time.Second) // 等待2秒后重试
				continue
			} else {
				log.Println("已达到最大重试次数，登录失败。")
				return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
			}
		}

		cookies := make(map[string]string)
		for _, cookie := range resp.Cookies() {
			cookies[cookie.Name] = cookie.Value
		}
		log.Printf("用户 %s 已成功登录", username)
		return cookies, nil
	}
	return nil, fmt.Errorf("达到最大重试次数，登录失败")
}

func getOrderData(target TargetConfig, cookies map[string]string, okAreaDict map[string]AreaInfo) (map[string]string, []byte) {
	headers := make(map[string]string)
	for k, v := range BASIC_HEADERS {
		headers[k] = v
	}

	// 合并 Cookies
	var cookieParts []string
	for key, value := range cookies {
		cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", key, value))
	}
	headers["Cookie"] = strings.Join(cookieParts, "; ")

	// 建立参数
	param := map[string]interface{}{
		"stockdetail": map[string]string{},
		"serviceid":   "1",
		"stockid":     "",
		"remark":      "",
	}

	// 目标场地名称
	targetName := fmt.Sprintf("场地%d", target.Number)
	// 目标时间字符串
	targetTimeStr := fmt.Sprintf("%02d:01-%02d:00", target.Time, target.Time+1)

	// 检查目标时间是否在现有场地中
	if _, ok := okAreaDict[targetTimeStr+targetName]; !ok {
		log.Println("目标时间不在现有场地中")
		return headers, nil
	}

	areaData := okAreaDict[targetTimeStr+targetName]
	param["stockdetail"].(map[string]string)[fmt.Sprintf("%d", areaData.StockID)] = fmt.Sprintf("%d", areaData.ID)
	param["stockid"] = fmt.Sprintf("%d,", areaData.StockID)

	// 创建 payload
	paramJSON, err := json.Marshal(param)
	if err != nil {
		log.Printf("JSON 序列化错误: %v\n", err)
		return headers, nil
	}
	payload := map[string]string{
		"param": string(paramJSON),
		"num":   "1",
		"json":  "true",
	}

	payloadJson, _ := json.Marshal(payload)

	return headers, payloadJson
}

func order(config Config, target TargetConfig, cookies map[string]string, okAreaDict map[string]AreaInfo) (bool, error) {
	orderHeaders, orderPayload := getOrderData(target, cookies, okAreaDict)
	if len(orderPayload) == 0 {
		return false, fmt.Errorf("获取预订数据失败")
	}

	req, err := http.NewRequest("POST", config.APIs.Pay, bytes.NewBuffer(orderPayload))
	fmt.Println(req)
	if err != nil {
		return false, fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	for key, value := range orderHeaders {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("发送HTTP请求失败: %v", err)
	}

	if err != nil {
		return false, fmt.Errorf("网络请求失败: %v", err)
	}
	var result struct {
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result.Message == "未支付" || result.Message == "预订成功" || result.Message == "本天预订数量超过限制" {
		return true, fmt.Errorf("预订成功: %s", result.Message)
	}
	return false, err
}

func main() {
	today := time.Now().Format("2006-01-02")
	config, err := getYAMLConfig("rootConfig.yml")
	if err != nil {
		log.Fatalf("读取配置文件时发生错误: %v", err)
	}
	log.Println("today:", today)
	// 1. find ok area
	okAreaMap, err := findOkArea(config.APIs.FindOkArea, today)
	log.Println("okAreaMap:", okAreaMap)
	if err != nil {
		log.Fatalf("获取空闲场地失败: %v", err)
	}
	// 2. get cookies
	cookies, err := GetCookies(config.APIs.Login, config.User.Username, config.User.Password)
	if err != nil {
		log.Fatalf("登录失败: %v", err)
	}
	// 3. order
	for _, target := range config.Target {
		if err != nil {
			log.Printf("order %v error: %v\n", target, err)
			continue
		}
		isOrder, error := order(config, target, cookies, okAreaMap)
		log.Printf("order %v result: %v, error: %v\n", target, isOrder, error)
	}
}
