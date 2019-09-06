// SoGouPack project main.go
package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	_ "regexp"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

//包中的词条如果超过2002条，只获取前2002条的
func main() {
	runtime.GOMAXPROCS(8)
	t1 := time.Now()
	//75228是词条包id
	//如下：https://pinyin.sogou.com/dict/detail/index/75228/?rf=dictindex&pos=slidebanner
	//75228就是"开发大神专用词库【官方推荐】"词条包的id
	//wors_list, title := GetSoGouWorkList("2651")
	title := ""
	//获取pack_content词条，这样可以无限制地转换
	wors_list := GetPackWords()

	fmt.Println(fmt.Sprintf("词包“%v”一共有%v个条词", title, len(wors_list)))
	channel := make(chan string, 10) //列队,10并行是为了保证请求的网址不拒绝请求
	poll := make(chan string, 10)    //池？ 控制http请求条数
	result := make(chan string)      //结果通知

	go func(wors_list []string) {
		for _, value := range wors_list {
			channel <- value
			poll <- value //当你写最大时，只有通知结果通知之后你才能继续走，否则停在这里，不写入队列消息
		}
	}(wors_list)

	//在此转换结果
	fmt.Println("转换中...")
	go func() {
		for {
			value := <-channel
			go func(value string) {
				content := strings.ReplaceAll(value, " ", "")
				fonts := strings.Split(content, "")
				fonts_len := len(fonts)
				all_code := "" //全码
				ju := false

				//分割单个汉字去查询
				for index, font := range fonts {
					if index > 3 && fonts_len-1 != index {
						continue
					}

					//转换词条，先通知百度接口，如果无法获取再去汉典接口查询；如果汉典接口也无法查询则跳过
					code := Get_Code_For_BaiDu(font) //百度接口
					if code == "" {
						code = Get_Code_For_HanDian(font) //汉典接口
					}

					//fmt.Println(font, "----", code)
					if code == "" {
						ju = true
						break
					}
					if fonts_len == 1 {
						all_code = code
					} else if fonts_len == 2 {
						all_code += code[:2]
					} else if fonts_len == 3 {
						if index < 2 {
							all_code += code[:1]
						} else {
							all_code += code[:2]
						}
					} else if fonts_len == 4 {
						all_code += code[:1]
					} else if fonts_len > 4 {
						if index < 3 {
							all_code += code[:1]
						} else if fonts_len-1 == index {
							all_code += code[:1]
						}
					}
				}
				if ju {
					result <- content + "暂无编码"
					fmt.Println(content + "暂无编码")
				} else {
					result <- fmt.Sprintf("%v\t%v\t1", all_code, content)
				}
			}(value)

		}

	}()

	//监听转换结果
	Result := 0
	var Lists []string
	for {
		item := <-result
		<-poll
		Result++
		Lists = append(Lists, item)
		fmt.Println(Result, "--", item)
		if len(wors_list) == Result {
			break
		}

	}

	//写入文件中
	font_new, _ := os.OpenFile("./转化结果.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
	defer font_new.Close()
	for _, value := range Lists {
		font_new.WriteString(value + "\n")
	}
	t2 := time.Now()
	fmt.Println(t2.Sub(t1))
	fmt.Println("处理完成")

}

//百度汉字查询接口
func Get_Code_For_BaiDu(str string) string {
	//fmt.Println("https://hanyu.baidu.com/zici/s?wd=" + str)
	defer func() {
		err := recover()
		if err != nil {
			fmt.Println("err:", err)
		}
	}()
	res, http_err := http.Get("https://hanyu.baidu.com/zici/s?wd=" + str)

	if http_err != nil {
		panic(http_err)
		return ""
	}
	defer res.Body.Close()
	el, el_err := goquery.NewDocumentFromReader(res.Body)
	if el_err != nil {
		fmt.Println(el_err)
		return ""
	}

	code := el.Find("#wubi span").Eq(0).Text()
	code = strings.ToLower(code)
	return code
}

//汉典
func Get_Code_For_HanDian(str string) string {
	defer func() {
		err := recover()
		if err != nil {
			fmt.Println("err:", err)
		}

	}()
	res, res_err := http.Get("https://www.zdic.net/hans/" + str)
	if res_err != nil {
		panic(res_err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	content := string(body)
	//fmt.Println(content)
	isHas := strings.Contains(content, "五笔")
	if isHas {
		el, el_err := goquery.NewDocumentFromReader(strings.NewReader(content))
		if el_err == nil {
			code := el.Find(" .dsk").Eq(2).Find("tr").Eq(1).Find("td").Find("p").First().Text()
			if strings.Contains(code, "|") {
				return code[strings.Index(code, "|")+1:]
			} else {
				return code
			}
		}
	}
	return ""
}

//获取搜狗词条,一个包只能获取2002条，因为搜狗网站不支持获取全部
func GetSoGouWorkList(id string) ([]string, string) {
	defer func() {
		err := recover()
		if err != nil {
			fmt.Println("获取词条失败", err)
		}

	}()
	res, res_err := http.Get("https://pinyin.sogou.com/dict/dialog/word_list/" + id)
	defer res.Body.Close()
	var word_list []string
	var title string
	if res_err == nil {
		el, el_err := goquery.NewDocumentFromReader(res.Body)
		if el_err == nil {
			el.Find("#words tr td div").Each(func(i int, s *goquery.Selection) {
				//因为是table,td当词库不足时，就为空
				if len(strings.ReplaceAll(s.Text(), " ", "")) > 0 {
					word_list = append(word_list, strings.ReplaceAll(s.Text(), " ", ""))
				}
			})

			title = el.Find(".poptbmidc div").Eq(0).Find("span").First().Text()
		} else {
			panic(res_err)
		}
	} else {
		panic(res_err)
	}
	return word_list, title
}

//打开pack_content并读取词条
func GetPackWords() []string {
	font_f, font_err := os.Open("./pack_content.txt")
	defer font_f.Close()
	//reg_nullor_line, _ := regexp.Compile(`\r?\n?`)
	var arrar []string
	if font_err == nil {
		rd := bufio.NewReader(font_f)
		for {
			line, _, err := rd.ReadLine()

			if err != nil || io.EOF == err {
				break
			}
			line_content := string(line)
			arrar = append(arrar, line_content)
		}
	}
	return arrar
}
