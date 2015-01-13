package xiaoi

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	REALM   = "xiaoi.com"
	METHOD  = "POST"
	URI     = "/ask.do"
	SCHEMA  = "http://"
	HOST    = "nlp." + REALM
	REQ_URL = SCHEMA + HOST + URI
)

const (
	THREAD_NUMBER = 8
	NONCE_SIZE    = 40
)

var (
	ErrQueueFull = errors.New("Queue full")
	SPLIT_BYTES  = []byte(":")
)

type XiaoiParameters struct {
	Param_key         string
	Param_secret      string
	Param_connections int
	Param_queue_size  int
	Param_timeout     time.Duration
}

type XiaoiMsg struct {
	Id       int64
	Userid   string
	Question string
	Answer   string
}

type Callback func(msg *XiaoiMsg)

type Xiaoi struct {
	XiaoiParameters
	queue    chan *XiaoiMsg
	id       int64
	callback Callback
	ha1      string
	ha2      string

	exitSg chan bool
	exitWg sync.WaitGroup
}

func NewXiaoi(parameters *XiaoiParameters, callback Callback) (xiaoi *Xiaoi) {
	xiaoi = &Xiaoi{
		XiaoiParameters: *parameters,
		queue:           make(chan *XiaoiMsg, parameters.Param_queue_size),
		exitSg:          make(chan bool, parameters.Param_connections),
		callback:        callback,
	}
	rand.Seed(time.Now().UnixNano())
	h := sha1.New()
	io.WriteString(h, xiaoi.Param_key)
	h.Write(SPLIT_BYTES)
	io.WriteString(h, REALM)
	h.Write(SPLIT_BYTES)
	io.WriteString(h, xiaoi.Param_secret)
	xiaoi.ha1 = fmt.Sprintf("%x", h.Sum(nil))
	fmt.Println("ha1:", xiaoi.ha1)
	h = sha1.New()
	io.WriteString(h, METHOD)
	h.Write(SPLIT_BYTES)
	io.WriteString(h, URI)
	xiaoi.ha2 = fmt.Sprintf("%x", h.Sum(nil))
	fmt.Println("ha2:", xiaoi.ha2)

	for i := 0; i < parameters.Param_connections; i++ {
		xiaoi.exitWg.Add(1)
		go xiaoi.loop()
	}
	return
}

func (xiaoi *Xiaoi) Post(userid string, question string) (id int64, err error) {
	if len(xiaoi.queue)+THREAD_NUMBER > xiaoi.Param_queue_size {
		return 0, ErrQueueFull
	}
	id = atomic.AddInt64(&xiaoi.id, 1)
	msg := &XiaoiMsg{
		Id:       id,
		Userid:   userid,
		Question: question,
	}
	xiaoi.queue <- msg
	return
}

func (xiaoi *Xiaoi) Close(timeout int) {
	for i := 0; i < xiaoi.Param_connections; i++ {
		xiaoi.exitSg <- true
	}
	go func() {
		time.Sleep(time.Duration(timeout) * time.Second)
		for i := 0; i < xiaoi.Param_connections; i++ {
			xiaoi.exitWg.Done()
		}
	}()
	xiaoi.exitWg.Wait()
}

func (xiaoi *Xiaoi) loop() {
FOR_LOOP:
	for {
		select {
		case msg := <-xiaoi.queue:
			xiaoi.doRequest(msg)
		case <-xiaoi.exitSg:
			break FOR_LOOP
		}
	}
	xiaoi.exitWg.Done()
}

func (xiaoi *Xiaoi) doRequest(msg *XiaoiMsg) {
	// 签名生成规则
	//所有小i机器人API的有效访问都必须包含签名请求头
	//签名算法如下：
	//1.sha1加密（app_key:realm:app_secret） 其中realm为"xiaoi.com"
	//2.sha1加密（method:uri） 其中method为请求方法，如"POST"， uri为"/ask.do"
	//3.sha1加密（HA1:nonce:HA2） 其中HA1为步骤1的值，HA2为步骤2的值，nonce为40位随机数
	//4.请求头字符串为app_key="app_key的值",nonce="nonce的值",signature="步骤3的值"
	//5.添加请求头"X-Auth"，值为步骤4的请求头字符串
	defer xiaoi.callback(msg)
	nonce := fmt.Sprintf("%016x%016x%08x", rand.Int63(), rand.Int63(), rand.Int63())
	h := sha1.New()
	io.WriteString(h, xiaoi.ha1)
	h.Write(SPLIT_BYTES)
	io.WriteString(h, nonce)
	h.Write(SPLIT_BYTES)
	io.WriteString(h, xiaoi.ha2)
	sign := fmt.Sprintf("%x", h.Sum(nil))
	authHeader := `app_key="` + xiaoi.Param_key + `",nonce="` + nonce + `",signature="` + sign + `"`
	fmt.Println("authHeader =", authHeader)

	body := new(bytes.Buffer)
	body.WriteString("userId=")
	body.WriteString(msg.Userid)
	body.WriteString("&question=")
	body.WriteString(msg.Question)
	body.WriteString("&type=0&platform=custom")
	fmt.Printf("body: %s\n", string(body.Bytes()))
	req, err := http.NewRequest(METHOD, REQ_URL, body)
	req.Header.Add("X-Auth", authHeader)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Http request err:", err.Error())
		return
	}
	if resp.StatusCode != http.StatusOk {
		fmt.Println("Http response:", resp.Status)
		return
	}
	answer, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Read response err:", err.Error())
		return
	}
	msg.Answer = string(answer)
}
