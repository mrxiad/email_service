package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Data 定义爬取的数据结构
type Data struct {
	Title string `json:"title"` // 标题
	Link  string `json:"link"`  // 链接
	Date  string `json:"date"`  // 日期
}

// Site 定义每个网站的配置结构
type Site struct {
	Name          string // 网站名称
	URL           string // 网站URL
	ListSelector  string // 列表的CSS选择器
	ItemSelector  string // 每个通知项的CSS选择器
	TitleSelector string // 标题的CSS选择器
	LinkSelector  string // 链接的CSS选择器（相对于每个通知项）
	DateSelector  string // 日期的CSS选择器
	BaseURL       string // 基础URL，用于构建绝对链接
}

// EmailConfig 定义邮件发送的配置结构
type EmailConfig struct {
	SMTPHost     string // SMTP服务器地址
	SMTPPort     string // SMTP服务器端口
	SMTPUsername string // SMTP用户名
	SMTPPassword string // SMTP密码
	FromEmail    string // 发件人邮箱
	ToEmail      string // 收件人邮箱（多个邮箱以逗号分隔）
	Subject      string // 邮件主题
}

// crawlSite 爬虫函数：爬取指定网站的信息
func crawlSite(site Site) ([]Data, error) {
	var results []Data

	// 发起HTTP GET请求
	resp, err := http.Get(site.URL)
	if err != nil {
		return results, fmt.Errorf("网站 %s HTTP请求失败: %v", site.Name, err)
	}
	defer resp.Body.Close()

	// 检查HTTP响应状态码
	if resp.StatusCode != 200 {
		return results, fmt.Errorf("网站 %s HTTP请求失败，状态码: %d %s", site.Name, resp.StatusCode, resp.Status)
	}

	// 使用goquery解析HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return results, fmt.Errorf("网站 %s 解析HTML失败: %v", site.Name, err)
	}

	// 选择包含通知列表的元素
	doc.Find(site.ListSelector).Each(func(i int, list *goquery.Selection) {
		// 在每个列表中查找每个通知项
		list.Find(site.ItemSelector).Each(func(j int, item *goquery.Selection) {
			// 提取标题
			title := item.Find(site.TitleSelector).Text()
			title = strings.TrimSpace(title)
			if title == "" {
				// 如果标题为空，跳过该元素
				return
			}

			// 提取链接
			link, exists := item.Find(site.LinkSelector).Attr("href")
			if !exists {
				// 如果没有href属性，跳过该元素
				return
			}

			// 构建绝对链接
			absoluteLink := link
			if strings.HasPrefix(link, "/") {
				absoluteLink = site.BaseURL + link
			}

			// 提取日期
			date := item.Find(site.DateSelector).Text()
			date = strings.TrimSpace(date)

			// 添加到结果切片
			results = append(results, Data{
				Title: title,
				Link:  absoluteLink,
				Date:  date,
			})
		})
	})

	return results, nil
}

// sendEmail 发送邮件函数：封装邮件发送逻辑
func sendEmail(config EmailConfig, body string) error {
	addr := fmt.Sprintf("%s:%s", config.SMTPHost, config.SMTPPort)

	// 设置邮件内容
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		config.FromEmail, config.ToEmail, config.Subject, body)

	// 认证
	auth := smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)

	// 发送邮件
	to := strings.Split(config.ToEmail, ",")
	return smtp.SendMail(addr, auth, config.FromEmail, to, []byte(message))
}

// sendEmailHTML 发送HTML格式邮件的函数
func sendEmailHTML(config EmailConfig, body string) error {
	// 设置SMTP服务器地址
	addr := fmt.Sprintf("%s:%s", config.SMTPHost, config.SMTPPort)

	// 设置邮件内容（HTML格式）
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n%s",
		config.FromEmail, config.ToEmail, config.Subject, body)

	// 认证
	auth := smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)

	// 发送邮件
	to := strings.Split(config.ToEmail, ",")
	return smtp.SendMail(addr, auth, config.FromEmail, to, []byte(message))
}

// storeData 存储函数：将数据存储到JSON文件，并返回新数据
func storeData(filename string, newData []Data) ([]Data, error) {
	// 确保目录存在
	dir := filepath.Dir(filename)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	// 读取现有数据
	var existingData []Data
	if _, err := os.Stat(filename); err == nil {
		file, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(file, &existingData); err != nil {
			return nil, err
		}
	}

	// 添加新数据并去重，返回仅新添加的数据
	updatedData, uniqueNewData := deduplicate(existingData, newData)

	// 序列化数据
	fileData, err := json.MarshalIndent(updatedData, "", "  ")
	if err != nil {
		return nil, err
	}

	// 写入文件
	if err := ioutil.WriteFile(filename, fileData, 0644); err != nil {
		return nil, err
	}

	return uniqueNewData, nil
}

// deduplicate 去重函数：根据链接去重，返回更新后的数据和仅新添加的数据
func deduplicate(existing, newData []Data) ([]Data, []Data) {
	existingMap := make(map[string]bool)
	for _, d := range existing {
		existingMap[d.Link] = true
	}

	var uniqueNewData []Data
	for _, d := range newData {
		if !existingMap[d.Link] {
			uniqueNewData = append(uniqueNewData, d)
			existingMap[d.Link] = true
		}
	}

	// 将新数据添加到现有数据中
	updatedData := append(existing, uniqueNewData...)

	return updatedData, uniqueNewData
}

func main() {

	if _, exists := os.LookupEnv("GITHUB_ACTIONS"); !exists {
		log.Fatalf("Error loading env")
		return
	}

	// 定义需要爬取的网站配置
	sites := []Site{
		{
			Name:          "教务处通知",
			URL:           "https://jwc.cugb.edu.cn/xszq/",
			ListSelector:  "ul.list_content",         // 根据最新HTML结构
			ItemSelector:  "li",                      // 每个通知项为<li>
			TitleSelector: "div.list_con_main",       // 标题在div.list_con_main
			LinkSelector:  "a",                       // 链接在<a>标签上
			DateSelector:  "div.list_con_time",       // 日期在div.list_con_time
			BaseURL:       "https://jwc.cugb.edu.cn", // 教务处的基础URL
		},
		{
			Name:          "信息工程学院公告",
			URL:           "https://sie.cugb.edu.cn/xygg/",
			ListSelector:  "ul#list_container",       // 根据实际HTML结构
			ItemSelector:  "li.list_cont_li",         // 每个通知项为<li>且具有特定类名
			TitleSelector: "div.list_i_r > a.p1",     // 标题在div.list_i_r > a.p1
			LinkSelector:  "div.list_i_r > a.p1",     // 链接在同一个<a>标签上
			DateSelector:  "div.list_i_r > p",        // 日期在div.list_i_r > p
			BaseURL:       "https://sie.cugb.edu.cn", // 信息工程学院公告的基础URL
		},
	}

	// 存储文件
	storageFile := "data.json"

	// 遍历每个网站，爬取数据
	var allNewData []Data
	for _, site := range sites {
		fmt.Printf("正在爬取网站: %s\n", site.Name)
		data, err := crawlSite(site)
		if err != nil {
			log.Printf("爬取网站 %s 失败: %v\n", site.Name, err)
			continue
		}

		if len(data) == 0 {
			fmt.Printf("网站 %s 没有找到任何数据。\n\n", site.Name)
			continue
		}

		// 存储数据并获取新数据
		newData, err := storeData(storageFile, data)
		if err != nil {
			log.Printf("存储网站 %s 数据失败: %v\n", site.Name, err)
			continue
		}

		if len(newData) == 0 {
			fmt.Printf("网站 %s 没有新的数据。\n\n", site.Name)
			continue
		}

		// 收集所有新数据
		allNewData = append(allNewData, newData...)

		// 打印爬取到的新数据
		for _, item := range newData {
			fmt.Printf("标题: %s\n", item.Title)
			fmt.Printf("链接: %s\n", item.Link)
			fmt.Printf("日期: %s\n", item.Date)
			fmt.Println("---------------------------")
		}

		fmt.Println()
	}

	// 如果没有新数据，结束程序
	if len(allNewData) == 0 {
		allNewData = []Data{
			{
				Title: "今日无新通知",
			},
		}
	}

	// 准备发送邮件
	// 从环境变量加载邮件配置
	emailConfig := EmailConfig{
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     os.Getenv("SMTP_PORT"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		FromEmail:    os.Getenv("FROM_EMAIL"),
		ToEmail:      os.Getenv("TO_EMAIL"),
		Subject:      "教务处信息推送",
	}

	fmt.Printf("emailConfig:%v\n", emailConfig)

	// 检查必要的邮件配置是否存在
	if emailConfig.SMTPHost == "" || emailConfig.SMTPPort == "" || emailConfig.SMTPUsername == "" ||
		emailConfig.SMTPPassword == "" || emailConfig.FromEmail == "" || emailConfig.ToEmail == "" {
		log.Println("邮件配置不完整，跳过发送邮件。")
		return
	}

	// 准备邮件正文
	var bodyBuilder strings.Builder
	bodyBuilder.WriteString("今日教务处通知：\n\n")
	for _, item := range allNewData {
		bodyBuilder.WriteString(fmt.Sprintf("标题: %s\n链接: %s\n日期: %s\n\n", item.Title, item.Link, item.Date))
	}

	// 发送邮件
	err := sendEmail(emailConfig, bodyBuilder.String())
	if err != nil {
		log.Printf("发送邮件失败: %v\n", err)
		return
	}

	fmt.Println("邮件已成功发送。")
}
