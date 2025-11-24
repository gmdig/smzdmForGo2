package smzdm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"regexp"

	"ggball.com/smzdm/file"
)

type result struct {
	ErrorCode string `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
	Data      Data   `json:"data"`
}

type Data struct {
	Rows  []Product `json:"rows"`
	Total int       `json:"total"`
}

type Product struct {
	ArticleTitle   string `json:"article_title"`
	ArticlePrice   string `json:"article_price"`
	ArticleWorthy  string `json:"article_worthy"`
	ArticleComment string `json:"article_comment"`
	ArticleId      string `json:"article_id"`
	ArticleDate    string `json:"publish_date_lt"`
	ArticlePic     string `json:"article_pic"`
	ArticleUrl     string `json:"article_url"`
	Referral       string `json:"article_referrals"`
}

// 全局配置
var globalConf = file.Config{}

// 推送信息文件地址
var pushedPath = "./pushed.json"

// 获取商品
func GetSatisfiedGoods(conf file.Config) ([]Product, []Product) {
	globalConf = conf
	fmt.Println("开始爬取符合条件商品。。")

	// 获取已推送文章id
	pushedMap := file.ReadPusedInfo(pushedPath)

	// 符合条件的商品集合
	var satisfyGoodsList []Product

	page := 0
	for {
		productList := GetGoods(page, "").Data.Rows

		if len(productList) > 0 {
			for _, good := range productList {
				// 评论数包含“K”时，默认给 1000
				if strings.Contains(strings.ToLower(good.ArticleComment), "k") {
					good.ArticleComment = "1000"
				}

				if removeByFilterRules(good, pushedMap) {
					continue
				}

				if satisfy(good, satisfyGoodsList) {
					satisfyGoodsList = append(satisfyGoodsList, good)
				}
			}
		}

		page++
		time.Sleep(time.Duration(2) * time.Second)

		if shouldStop(len(satisfyGoodsList), page) {
			fmt.Println("退出")
			break
		}
	}

	// 评论数排序
	sort.SliceStable(satisfyGoodsList, func(a, b int) bool {
		return strings.Compare(satisfyGoodsList[a].ArticleComment, satisfyGoodsList[b].ArticleComment) > 0
	})

	fmt.Println("结束爬取符合条件商品。。")

	// 自己的商品
	satisfyGoodsListBySelf := filterMyselfProduct(satisfyGoodsList)

	// 保存推送商品
	savePushed(pushedMap, pushedPath, satisfyGoodsList)

	return satisfyGoodsList, satisfyGoodsListBySelf
}

// 获取商品集合
func GetGoods(page int, keword string) result {
	var res result

	params := url.Values{}
	Url, err := url.Parse("https://api.smzdm.com/v1/list")
	if err != nil {
		return res
	}
	params.Set("keyword", keword)
	params.Set("order", "time")
	params.Set("type", "good_price")
	params.Set("offset", strconv.Itoa(page*100))
	params.Set("limit", "100")

	Url.RawQuery = params.Encode()
	urlPath := Url.String()
	fmt.Println(urlPath)

	// 使用带超时的 http.Client 并设置常见请求头（最小改动以避免被远端拒绝或请求长时间阻塞）
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest("GET", urlPath, nil)
	if err != nil {
		log.Printf("构建请求失败: %v\n", err)
		return res
	}
	// 设置 User-Agent 与 Accept，保留原有行为同时减少被屏蔽的几率
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; smzdmForGo/1.0; +https://github.com/gmdig/smzdmForGo2)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// 网络层错误或超时
		log.Printf("HTTP 请求失败: %v\n", err)
		return res
	}
	defer resp.Body.Close()

	// 打印 HTTP 状态，便于诊断被封/重定向/认证等问题
	log.Printf("HTTP %s -> %s\n", urlPath, resp.Status)

	body, _ := ioutil.ReadAll(resp.Body)

	// 如果响应不是 200，打印 body 的前几百字节以帮助定位
	if resp.StatusCode != 200 {
		preview := string(body)
		if len(preview) > 1000 {
			preview = preview[:1000]
		}
		log.Printf("非 200 响应 (%d). body preview: %s\n", resp.StatusCode, preview)
		return res
	}

	// 反序列化，并在反序列化错误或 Data.Rows 为空时记录 body 以便排查
	if err := json.Unmarshal(body, &res); err != nil {
		preview := string(body)
		if len(preview) > 2000 {
			preview = preview[:2000]
		}
		log.Printf("JSON Unmarshal 错误: %v\nbody preview: %s\n", err, preview)
		return res
	}

	// 如果成功解析但没有 rows，打印 body preview（便于确认 API 返回了空列表还是结构变化）
	if len(res.Data.Rows) == 0 {
		preview := string(body)
		if len(preview) > 1000 {
			preview = preview[:1000]
		}
		log.Printf("解析成功但 rows 为空。body preview: %s\n", preview)
	}

	return res
}

// 根据条件 判断是否应该停止爬取
func shouldStop(length int, page int) bool {
	fmt.Println("length:" + strconv.Itoa(length) + "\n\r page:" + strconv.Itoa(page))
	return length > globalConf.SatisfyNum || page > 100
}

// 根据过滤规则，去除商品
func removeByFilterRules(good Product, pushedMap map[string]interface{}) bool {
	var noNeed = false

	// 1. 标题/价格包含过滤词
	for _, word := range globalConf.FilterWords {
		var pattern string
		if strings.HasPrefix(word, "re:") {
			pattern = "(?i)" + word[3:]
		} else {
			pattern = "(?i)" + regexp.QuoteMeta(word)
		}

		if matched, _ := regexp.MatchString(pattern, good.ArticleTitle); matched {
			fmt.Printf("过滤掉(标题): %s by %s\n", good.ArticleTitle, word)
			noNeed = true
			break
		}
		if matched, _ := regexp.MatchString(pattern, good.ArticlePrice); matched {
			fmt.Printf("过滤掉(价格): %s by %s\n", good.ArticlePrice, word)
			noNeed = true
			break
		}
	}

	// 2. 已推送过
	if _, exists := pushedMap[good.ArticleId]; exists {
		noNeed = true
	}

	// 3. 时间小于前天
	nTime := time.Now()
	beforeYesDate := nTime.AddDate(0, 0, -2)
	dateInt64, err1 := strconv.ParseInt(good.ArticleDate, 10, 64)
	if err1 != nil {
		panic(err1)
	}
	arDate := time.Unix(dateInt64, 0)
	if arDate.Before(beforeYesDate) {
		noNeed = true
	}

	return noNeed
}

// 根据规则判断符合规则的商品
func satisfy(good Product, satisfyGoodsList []Product) bool {
	articleComment, err1 := strconv.Atoi(good.ArticleComment)
	articleWorthy, err2 := strconv.Atoi(good.ArticleWorthy)

	if err1 != nil || err2 != nil {
		fmt.Println("goods:", good)
		panic(err1)
	}

	if articleComment >= globalConf.LowCommentNum || articleWorthy >= globalConf.LowWorthyNum {
		fmt.Printf("appear satisfy good: %#v", good)
		return true
	}
	return false
}

// 保存推送商品
func savePushed(pushedMap map[string]interface{}, pushedPath string, satisfyGoodsList []Product) {
	tempMap := make(map[string]interface{})
	for index, value := range satisfyGoodsList {
		tempMap[value.ArticleId] = index
	}
	file.WritePushedInfo(tempMap, pushedMap, pushedPath)
}

// 过滤自己的商品
func filterMyselfProduct(satisfyGoodsList []Product) []Product {
	var satisfyGoodsListBySelf []Product

	for _, value := range satisfyGoodsList {
		for _, word := range globalConf.KeyWords {
			var pattern string
			if strings.HasPrefix(word, "re:") {
				pattern = "(?i)" + word[3:]
			} else {
				pattern = "(?i)" + regexp.QuoteMeta(word)
			}

			matched, err := regexp.MatchString(pattern, value.ArticleTitle)
			if err != nil {
				fmt.Printf("正则表达式匹配错误: %v (word=%s)\n", err, word)
				continue
			}
			if matched {
				fmt.Printf("appear myself satisfy good: %#v\n", value)
				satisfyGoodsListBySelf = append(satisfyGoodsListBySelf, value)
				break
			}
		}
	}
	return satisfyGoodsListBySelf
}