package main

import (
	"cave-archive/internal/application"
	"cave-archive/internal/httpapi"
	"cave-archive/internal/store"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func listenAddr(flagAddr string) (string, error) {
	addr := flagAddr
	if addr == "" {
		if p := os.Getenv("PORT"); p != "" {
			if strings.Contains(p, ":") {
				addr = p
			} else {
				addr = "127.0.0.1:" + p
			}
		}
	}
	if addr == "" {
		addr = defaultAddr()
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		return "", fmt.Errorf("监听地址必须为127.0.0.1回环地址")
	}
	return addr, nil
}
func main() {
	addrFlag := flag.String("addr", "", "监听地址")
	self := flag.Bool("selfcheck", false, "运行自检")
	flag.Parse()
	addr, e := listenAddr(*addrFlag)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	st, e := store.New("data/archive-events.jsonl")
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	app := application.New(st)
	srv := httpapi.New(app)
	if *self {
		if e := selfcheck(addr, srv.Handler()); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	server := &http.Server{Addr: addr, Handler: srv.Handler()}
	fmt.Println("服务监听 " + addr)
	if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func selfcheck(addr string, h http.Handler) error {
	server := &http.Server{Addr: addr, Handler: h}
	go server.ListenAndServe()
	client := &http.Client{Timeout: 3 * time.Second}
	var e error
	for i := 0; i < 30; i++ {
		time.Sleep(20 * time.Millisecond)
		_, e = client.Get("http://" + addr + "/")
		if e == nil {
			break
		}
	}
	if e != nil {
		return e
	}
	body := fmt.Sprintf("{\"archiveCode\":\"SC-%d\",\"caveName\":\"自检洞\",\"surveyDate\":\"2024-01-01\",\"coordinateDatum\":\"WGS84\"}", time.Now().UnixNano())
	req, _ := http.NewRequest("POST", "http://"+addr+"/api/archives", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return fmt.Errorf("创建自检归档失败: %s", resp.Status)
	}
	var v map[string]any
	if e = json.NewDecoder(resp.Body).Decode(&v); e != nil {
		return e
	}
	id, _ := v["id"].(string)
	if id == "" {
		return fmt.Errorf("缺少归档ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}
