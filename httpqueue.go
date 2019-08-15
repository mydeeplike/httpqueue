package httpqueue

import (
	"encoding/json"
	"fmt"
	"github.com/mydeeplike/dbx"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//
//type Headers struct {
//	Data map[string]string
//}
//
//func NewHeaders() *Headers {
//	h := &Headers{}
//	h.Data = make(map[string]string)
//	return h
//}
//
//func (h *Headers) Set(k string, v string) {
//	h.Data[k] = v
//}
//
//func (h *Headers) Reset() {
//	h.Data = make(map[string]string)
//}
//
//func (h *Headers) String() string {
//	s := ""
//	sep := ""
//	for k, v := range h.Data {
//		s += fmt.Sprintf("%v:%v%v", k, v, sep)
//		sep = "\n"
//	}
//	return s
//}
//
//func (h *Headers) FromString(s string) {
//	h.Reset()
//	arr := strings.Split(s, "\n")
//	for _, v := range arr {
//		arr2 := strings.Split(v, ":")
//		if len(arr2) < 2 {
//			continue
//		}
//		h.Data[arr2[0]] = arr2[1]
//	}
//}

type Request struct {
	Id          int    `db:"id"`
	Url         string `db:"url"`
	Method      string `db:"method"`
	Post        string `db:"post"`
	ContentType string `db:"content_type"`
	//Headers      string    `db:"headers"`
	Retrys       int       `db:"retrys"`
	SuccessStr   string    `db:"success_str"`
	LastResponse string    `db:"last_response"`
	LastDate     time.Time `db:"last_date"`
}

type HttpQueue struct {
	db *dbx.DB
	//headers     *Headers
	contentType string
	retrys int
}

const (
	APPLICATION_X_WWW_FORM_URL_ENCODED = iota
	APPLICATION_JSON
	APPLICATION_MULTIPART_FORM_DATA
)

var ContentTypes = []string{
	"application/x-www-form-url-encoded",
	"application/json",
	"application/multipart-form-data",
}

// 发送 POST 数据
// SetHeader("Content-Type", "application/x-www-form-urlencoded")
//func (h *HttpQueue) SetHeader(k string, v string) {
//	h.headers.Set(k, v)
//}

//func (h *HttpQueue) SetContentType(t int) {
//	if t >= len(ContentTypes) {
//		t = 0
//	}
//	//h.headers.Set("content-type", ContentTypes[t])
//}

//func (h *HttpQueue) ResetHeader() {
//	h.headers.Reset()
//}

func EncodePost(post map[string]string, contentType int) string {
	// 根据 SetHeader("Content-Type")
	s := ""
	sep := ""
	if contentType == APPLICATION_X_WWW_FORM_URL_ENCODED {
		for k, v := range post {
			s += fmt.Sprintf("%v=%v%v", k, url.QueryEscape(v), sep)
			sep = "&"
		}
	} else if contentType == APPLICATION_JSON {
		arr, err := json.Marshal(post)
		if err == nil {
			s = string(arr)
		}
	}
	return s
}

func (h *HttpQueue) POST(url string, post string, contentType int, successStr string) error {
	contentTypeStr := ContentTypes[contentType]
	req := &Request{
		Url:          url,
		Post:         post,
		Method:       "POST",
		ContentType:  contentTypeStr,
		Retrys:       0,
		SuccessStr: successStr,
		LastResponse: "",
		LastDate:     time.Now(),
	}
	_, err := h.db.Table("request").Insert(req)
	return err
}

func (h *HttpQueue) GET(url string, successStr string) error {
	req := &Request{
		Url:          url,
		Post:         "",
		Method:       "GET",
		ContentType:  "",
		Retrys:       0,
		SuccessStr: successStr,
		LastResponse: "",
		LastDate:     time.Now(),
	}
	_, err := h.db.Table("request").Insert(req)
	return err
}

// new 一个队列, retrys 重试次数，-1 为无穷多次。
func New(dbPath string, retrys int) *HttpQueue {
	var err error
	h := &HttpQueue{}
	h.retrys = retrys
	h.db, err = dbx.Open("sqlite3", dbPath+"?cache=shared&mode=rwc&parseTime=true&charset=utf8")
	if err != nil {
		panic(err)
	}
	//defer h.db.Close()
	h.db.SetMaxIdleConns(1)
	h.db.SetMaxOpenConns(1)
	h.db.SetConnMaxLifetime(time.Second * 5) // 不使用的链接关闭掉，快速回收。
	_, err = h.db.Exec(`
		DROP TABLE IF EXISTS request;
		CREATE TABLE IF NOT EXISTS request (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL default '',
			method VARCHAR(255) NOT NULL default '',
			post TEXT NOT NULL default '',
			content_type VARCHAR(255) NOT NULL default '',
			headers TEXT NOT NULL default '',
			retrys INT NOT NULL DEFAULT '0',
			success_str VARCHAR(255) NOT NULL default '',
			last_response TEXT NOT NULL default '',
			last_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		DROP TABLE IF EXISTS request_success;
		CREATE TABLE IF NOT EXISTS request_success (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL default '',
			method VARCHAR(255) NOT NULL default '',
			post TEXT NOT NULL default '',
			content_type VARCHAR(255) NOT NULL default '',
			headers TEXT NOT NULL default '',
			retrys INT NOT NULL DEFAULT '0',
			success_str VARCHAR(255) NOT NULL default '',
			last_response TEXT NOT NULL default '',
			last_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		DROP TABLE IF EXISTS request_failed;
		CREATE TABLE IF NOT EXISTS request_failed (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL default '',
			method VARCHAR(255) NOT NULL default '',
			post TEXT NOT NULL default '',
			content_type VARCHAR(255) NOT NULL default '',
			headers TEXT NOT NULL default '',
			retrys INT NOT NULL DEFAULT '0',
			success_str VARCHAR(255) NOT NULL default '',
			last_response TEXT NOT NULL default '',
			last_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		panic(err)
	}
	h.db.Bind("request", &Request{}, true)
	h.db.EnableCache(true)

	// 启动一个协程不停的扫描队列
	go func() {
		var ret string
		var err error
		var n int64
		for {
			n, err = h.db.Table("request").Count()
			if err != nil {
				log.Print(err)
				time.Sleep(1 * time.Second)
				continue
			}
			if n == 0 {
				time.Sleep(1 * time.Second)
				continue
			}

			req := new(Request)
			err = h.db.Table("request").One(req)
			if err != nil {
				log.Print(err)
				goto End
			}

			if req.Method == "POST" {
				ret, err = HttpPost(req.Url, req.ContentType, req.Post)
				if err != nil {
					log.Print(err)
					goto End
				}
			} else {
				ret, err = HttpGet(req.Url)
				if err != nil {
					log.Print(err)
					goto End
				}
			}

			// 如果成功，赶紧下一条
			if strings.Contains(ret, req.SuccessStr) {
				req.LastResponse = ret
				h.db.Table("request_success").Insert(req)
				h.db.Table("request").WherePK(req.Id).Delete()
				continue
			} else {
				req.LastDate = time.Now()
				req.LastResponse = ret
				req.Retrys++
				_, err = h.db.Table("request").Update(req)
				if err != nil {
					log.Print(err)
					goto End
				}

				if h.retrys > -1 && req.Retrys > h.retrys {
					h.db.Table("request_failed").Insert(req)
					h.db.Table("request").WherePK(req.Id).Delete()
				}

			}
			End:
			time.Sleep(1 * time.Second)
		}
	}()

	return h
}

func HttpPost(url string, contentType, data string, args ...interface{}) (body string, err error) {
	retry := 0
	if len(args) > 0 {
		retry = args[0].(int)
	}

Flag1:
	resp, err := http.Post(url, contentType, strings.NewReader(data))
	if err != nil {
		if retry > 0 {
			retry--
			goto Flag1
		}
		return "", err
	}
	defer resp.Body.Close()
	bodyByte, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if retry > 0 {
			retry--
			goto Flag1
		}
		return "", err
	}
	return string(bodyByte), nil
}

func HttpGet(url string, args ...interface{}) (string, error) {
	retry := 0
	if len(args) > 0 {
		retry = args[0].(int)
	}

Flag1:
	timeout := time.Duration(60 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)

	if err != nil {
		if retry > 0 {
			retry--

			goto Flag1
		}
		return "", err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		if retry > 0 {
			retry--
			goto Flag1
		}
		return "", err
	}
	return string(body), nil
}
