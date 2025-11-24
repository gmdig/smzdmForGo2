package file

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// 配置文件（保留旧字段以兼容现有代码）
// 为了尽量减少改动：不在 Config 中声明复杂的 Keywords 结构，
// 仅保持旧的 KeyWords([]string)、全局字段和其他需要的字段。
// 读取时对新的 keywords 配置做预处理，抽取 keyword 字符串填充到 KeyWords。
type Config struct {
	LowCommentNum int     `yaml:"lowCommentNum"`
	MaxPrice      float64 `yaml:"maxPrice"`
	MinPrice      float64 `yaml:"minPrice"`
	LowWorthyNum  int     `yaml:"lowWorthyNum"`

	SatisfyNum       int      `yaml:"satisfyNum"`
	TickTime         int      `yaml:"tickTime"`
	FilterWords      []string `yaml:"filterWords"`
	KeyWords         []string `yaml:"keyWords"`
	DingdingToken    string   `yaml:"dingdingToken"`
	Cron             string   `yaml:"cron"`
	TelegramBotToken string   `yaml:"telegramBotToken"`
	TelegramChatID   string   `yaml:"telegramChatId"`
}

// 签到信息
type CheckInfo struct {
	Id         int    `json:Id`
	LastTIme   string `json:LastTIme`
	Remark     string `json:Remark`
	LastMsg    string `json:LastMsg`
	LastResult string `json:LastResult`
	Cookie     string `json:Cookie`
}

// 读取已推送文章id 返回map
func ReadPusedInfo(path string) map[string]interface{} {
	jsonFile, err := os.Open(path)
	if err != nil {
		// 如果文件不存在则创建
		if os.IsNotExist(err) {
			jsonFile, err = os.Create(path)
			if err != nil {
				panic(err)
			}
			// 写入空的json对象
			_, err = jsonFile.Write([]byte("{}"))
			if err != nil {
				panic(err)
			}
			// 将文件指针移到开头
			jsonFile.Seek(0, 0)
		} else {
			panic(err)
		}
	}
	defer jsonFile.Close()

	bytesFile, _ := ioutil.ReadAll(jsonFile)

	pushedMap := make(map[string]interface{})
	err1 := json.Unmarshal(bytesFile, &pushedMap)
	if err1 != nil {
		panic(err1)
	}
	return pushedMap
}

// 保存已推送文章id 到本地
func WritePushedInfo(temp map[string]interface{}, pushed map[string]interface{}, path string) {
	for key, value := range temp {
		pushed[key] = value
	}

	// 长度大于5000 则删除头部的1000个数据
	if len(pushed) > 5000 {
		for i := 0; i < 1000; i++ {
			delete(pushed, fmt.Sprintf("%d", i))
		}
	}

	// json 序列化map
	data, _ := json.Marshal(pushed)

	err := ioutil.WriteFile(path, data, 0644)
	if err != nil {
		panic(err)
	}
}

// 读取配置文件（最小改动）
// - 兼容旧的 keyWords: []string
// - 支持新的 keywords: - keyword: "..."（只提取 keyword 字符串并填充到 keyWords）
// 这样可以避免 viper.Unmarshal 因 Keywords（复杂结构）而报类型错误。
// 保留对 "re:" 前缀正则形式的支持（不会修改关键词字符串内容）。
func ReadConf(pwd string) Config {

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// 保持原行为：如果传入非空字符串，将其当作工作目录（最小改动）
	if pwd != "" {
		wd = pwd
	}

	cnf := Config{}
	c := &cnf
	v := viper.New()

	// 配置文件路径与原来一致
	path := filepath.Join(wd, "config", "config.yml")
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		log.Fatal("读取配置文件失败：", err)
		return cnf
	}

	// 把读取到的设置作为默认（保留原逻辑）
	configs := v.AllSettings()
	for k, val := range configs {
		v.SetDefault(k, val)
	}

	// 1) 从 new keywords 中抽取 keyword 字符串到 keyWords/KeyWords（兼容大小写）
	//    支持以下情况：
	//    - keywords: - keyword: "abc"  (map entries)
	//    - keywords: ["a","b"]        (string entries)
	if v.IsSet("keywords") {
		raw := v.Get("keywords")
		if arr, ok := raw.([]interface{}); ok {
			kws := []string{}
			for _, it := range arr {
				switch m := it.(type) {
				case map[string]interface{}:
					if kwv, ok := m["keyword"]; ok {
						if s, ok := kwv.(string); ok && s != "" {
							kws = append(kws, s)
						}
					}
				case map[interface{}]interface{}:
					if kwv, ok := m["keyword"]; ok {
						if s, ok := kwv.(string); ok && s != "" {
							kws = append(kws, s)
						}
					}
				case string:
					if m != "" {
						kws = append(kws, m)
					}
				}
			}
			if len(kws) > 0 {
				v.Set("keyWords", kws)
				v.Set("KeyWords", kws)
			}
		} else if arr2, ok := raw.([]string); ok {
			if len(arr2) > 0 {
				v.Set("keyWords", arr2)
				v.Set("KeyWords", arr2)
			}
		} else if s, ok := raw.(string); ok && s != "" {
			// 单字符串的情况下也支持
			v.Set("keyWords", []string{s})
			v.Set("KeyWords", []string{s})
		}
	} else {
		// 2) 若没有 keywords 节点，则尽量确保 keyWords/KeyWords（旧字段）为字符串数组
		var kws []string
		collect := func(val interface{}) {
			switch a := val.(type) {
			case []interface{}:
				for _, it := range a {
					if s, ok := it.(string); ok && s != "" {
						kws = append(kws, s)
					}
				}
			case []string:
				for _, s := range a {
					if s != "" {
						kws = append(kws, s)
					}
				}
			case string:
				if a != "" {
					kws = append(kws, a)
				}
			}
		}
		if v.IsSet("keyWords") {
			collect(v.Get("keyWords"))
		}
		if v.IsSet("KeyWords") {
			collect(v.Get("KeyWords"))
		}
		if len(kws) > 0 {
			v.Set("keyWords", kws)
			v.Set("KeyWords", kws)
		}
	}

	// 反序列化至结构体（此处 Config 不包含复杂 Keywords 字段，避免类型冲突）
	if err := v.Unmarshal(c); err != nil {
		log.Fatal("读取配置错误：", err)
	}

	// 额外：若存在 globalDefaults 节点，从中回填旧字段（最小改动）
	if v.IsSet("globalDefaults") {
		raw := v.Get("globalDefaults")
		switch m := raw.(type) {
		case map[string]interface{}:
			if vvv, ok := m["lowCommentNum"]; ok {
				if n, ok := vvv.(int); ok {
					if c.LowCommentNum == 0 {
						c.LowCommentNum = n
					}
				} else if f, ok := vvv.(float64); ok { // YAML 数字可能被解析为 float64
					if c.LowCommentNum == 0 {
						c.LowCommentNum = int(f)
					}
				}
			}
			if vvv, ok := m["lowWorthyNum"]; ok {
				if n, ok := vvv.(int); ok {
					if c.LowWorthyNum == 0 {
						c.LowWorthyNum = n
					}
				} else if f, ok := vvv.(float64); ok {
					if c.LowWorthyNum == 0 {
						c.LowWorthyNum = int(f)
					}
				}
			}
			if vvv, ok := m["filterWords"]; ok {
				if arr, ok := vvv.([]interface{}); ok {
					if len(c.FilterWords) == 0 {
						for _, it := range arr {
							if s, ok := it.(string); ok && s != "" {
								c.FilterWords = append(c.FilterWords, s)
							}
						}
					}
				} else if arr2, ok := vvv.([]string); ok {
					if len(c.FilterWords) == 0 {
						c.FilterWords = append(c.FilterWords, arr2...)
					}
				}
			}
		case map[interface{}]interface{}:
			if vvv, ok := m["lowCommentNum"]; ok {
				switch t := vvv.(type) {
				case int:
					if c.LowCommentNum == 0 {
						c.LowCommentNum = t
					}
				case float64:
					if c.LowCommentNum == 0 {
						c.LowCommentNum = int(t)
					}
				}
			}
			if vvv, ok := m["lowWorthyNum"]; ok {
				switch t := vvv.(type) {
				case int:
					if c.LowWorthyNum == 0 {
						c.LowWorthyNum = t
					}
				case float64:
					if c.LowWorthyNum == 0 {
						c.LowWorthyNum = int(t)
					}
				}
			}
			if vvv, ok := m["filterWords"]; ok {
				if arr, ok := vvv.([]interface{}); ok {
					if len(c.FilterWords) == 0 {
						for _, it := range arr {
							if s, ok := it.(string); ok && s != "" {
								c.FilterWords = append(c.FilterWords, s)
							}
						}
					}
				}
			}
		}
	}

	// 合理默认值，避免未设置导致运行时问题
	if c.FilterWords == nil {
		c.FilterWords = []string{}
	}
	if c.LowCommentNum == 0 {
		c.LowCommentNum = 1
	}
	if c.LowWorthyNum == 0 {
		c.LowWorthyNum = 6
	}
	if c.SatisfyNum == 0 {
		c.SatisfyNum = 50
	}
	if c.TickTime == 0 {
		c.TickTime = 300
	}

	fmt.Print("读取配置文件成功。。")
	return cnf
}

func ReadPathConf(path string) Config {

	_, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cnf := Config{}
	c := &cnf
	v := viper.New()
	v.SetConfigName("config") //这里就是上面我们配置的文件名称，不需要带后缀名
	v.AddConfigPath(path)     //文件所在的目录路径
	v.SetConfigType("yml")    //这里是文件格式类型

	err = v.ReadInConfig()
	if err != nil {
		log.Fatal("读取配置文件失败：", err)
		return cnf
	}
	configs := v.AllSettings()
	for k, val := range configs {
		v.SetDefault(k, val)
	}
	err = v.Unmarshal(c) //反序列化至结构体
	if err != nil {
		log.Fatal("读取配置错误：", err)
	}
	fmt.Print("读取配置文件成功。。")
	return cnf
}

func UpdateCheckInfoById(id int, resultCode string, resultMsg string) {

	// 读取checkInfo 数据
	wd, err := os.Getwd()
	// 打开json文件
	jsonFile, err := os.Open("" + wd + "/template/json/checkInfo.json")

	// 最好要处理以下错误
	if err != nil {
		fmt.Println(err)
	}

	// 要记得关闭
	defer jsonFile.Close()

	checksByte, _ := ioutil.ReadAll(jsonFile)

	// 转为数组checkInfo
	checks := DeserializeJson(string(checksByte))
	// 更新最近一次签到结果
	for index, info := range checks {
		if info.Id == id {
			checks[index].LastMsg = resultMsg
			checks[index].LastResult = resultCode
			checks[index].LastTIme = time.Now().Format("2006-01-02 15:04:05")
		}
	}
	fmt.Println(checks)
	// 保存
	WriteCheckInfoJson(checks)

}

func DeserializeJson(CheckInfoJson string) []CheckInfo {
	// fmt.Println("CheckInfoJson:", CheckInfoJson)
	jsonAsBytes := []byte(CheckInfoJson)
	checks := make([]CheckInfo, 0)
	err := json.Unmarshal(jsonAsBytes, &checks)
	// fmt.Printf("%#v", checks)
	if err != nil {
		panic(err)
	}
	return checks
}

func WriteCheckInfoJson(chekInfos []CheckInfo) {
	// 互斥锁
	var mutex sync.Mutex

	mutex.Lock()
	// 读取checkInfo 数据
	wd, error := os.Getwd()
	if error != nil {
		fmt.Println(error)
	}

	data, _ := json.Marshal(chekInfos)

	err := ioutil.WriteFile(""+wd+"/template/json/checkInfo.json", data, 0644)
	if err != nil {
		panic(err)
	}
	mutex.Unlock()
}

func ReadCheckInfoJsonToByte() []byte {

	wd, error := os.Getwd()
	if error != nil {
		fmt.Println(error)
	}
	// 打开json文件
	jsonFile, err := os.Open("" + wd + "/template/json/checkInfo.json")

	// 最好要处理以下错误
	if err != nil {
		fmt.Println(err)
	}

	// 要记得关闭
	defer jsonFile.Close()

	jsonByte, _ := ioutil.ReadAll(jsonFile)
	return jsonByte
}

func ReadCheckInfoJsonToCheck() []CheckInfo {

	wd, err := os.Getwd()

	// 打开json文件
	jsonFile, err := os.Open("" + wd + "/template/json/checkInfo.json")

	// 最好要处理以下错误
	if err != nil {
		fmt.Println(err)
	}

	// 要记得关闭
	defer jsonFile.Close()

	checksByte, _ := ioutil.ReadAll(jsonFile)
	return DeserializeJson(string(checksByte))

}