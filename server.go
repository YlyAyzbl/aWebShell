package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// HTML页面内容
const htmlPage = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <title>WebShell</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@4.14.1/css/xterm.css" />
  <style>html, body {margin:0; padding:0; height:100%;} #terminal {width:100%; height:100%;}</style>
</head>
<body>
<div id="terminal"></div>
<script src="https://cdn.jsdelivr.net/npm/xterm@4.14.1/lib/xterm.js"></script>
<script>
  const term = new Terminal({ cursorBlink: true });
  term.open(document.getElementById('terminal'));

  const protocol = (location.protocol === 'https:') ? 'wss://' : 'ws://';
  const socketUrl = protocol + window.location.host + '/ws';
  const socket = new WebSocket(socketUrl);

  socket.onmessage = (event) => term.write(event.data);
  socket.onopen = () => term.write("Connected.\r\n");
  socket.onclose = () => term.write("Disconnected.\r\n");

  term.onData(data => {
    // 把所有字符(包括 Ctrl+C 等)原封不动发往后端
    socket.send(data);
  });
</script>
</body>
</html>`

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境中应该更严格
	},
}

// 首页处理器
func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlPage))
}

// WebSocket处理器
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// 创建一个新的bash进程
	cmd := exec.Command("/bin/sh")

	// 使用pty创建伪终端
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start pty: %v", err)
		return
	}
	defer func() {
		ptmx.Close()
		cmd.Process.Kill()
	}()

	// 从pty读取数据并发送到WebSocket的goroutine
	go func() {
		buffer := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from pty: %v", err)
				}
				return
			}

			err = conn.WriteMessage(websocket.TextMessage, buffer[:n])
			if err != nil {
				log.Printf("Error writing to websocket: %v", err)
				return
			}
		}
	}()

	// 处理来自WebSocket的消息并写入pty
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading from websocket: %v", err)
			break
		}

		// 将接收到的数据（包括Ctrl+C等控制字符）直接写入pty
		_, err = ptmx.Write(message)
		if err != nil {
			log.Printf("Error writing to pty: %v", err)
			break
		}
	}
}

func main() {
	// 设置信号处理
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// 设置路由
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/ws", websocketHandler)

	// 启动服务器
	go func() {
		fmt.Println("Starting WebShell server on :5000")
		if err := http.ListenAndServe(":5000", nil); err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()

	// 等待信号
	<-c
	fmt.Println("\nShutting down server...")
}
