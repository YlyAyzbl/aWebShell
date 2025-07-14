package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const htmlPage = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WebShell</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/xterm@4.14.1/css/xterm.css" />
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            height: 100vh;
            overflow: hidden;
        }
        
        #header {
            background: rgba(0, 0, 0, 0.1);
            backdrop-filter: blur(10px);
            color: white;
            padding: 15px 20px;
            text-align: center;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }
        
        #header h1 {
            font-size: 24px;
            font-weight: 300;
            letter-spacing: 2px;
        }
        
        #container {
            display: flex;
            height: calc(100vh - 70px);
            padding: 20px;
            gap: 20px;
        }
        
        #terminal-wrapper {
            flex: 2;
            background: #1e1e1e;
            border-radius: 12px;
            overflow: hidden;
            box-shadow: 0 15px 35px rgba(0, 0, 0, 0.3);
            border: 1px solid rgba(255, 255, 255, 0.1);
            position: relative;
            display: flex;
            flex-direction: column;
        }
        
        #terminal-wrapper .xterm {
            width: 100% !important;
            height: 100% !important;
            padding: 10px;
        }
        
        #terminal-wrapper .xterm-viewport {
            width: 100% !important;
        }
        
        #sidebar {
            flex: 1;
            display: flex;
            flex-direction: column;
            gap: 20px;
            min-width: 300px;
        }
        
        #file-list-container {
            flex: 1;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 15px 35px rgba(0, 0, 0, 0.1);
            border: 1px solid rgba(255, 255, 255, 0.2);
            overflow: hidden;
            display: flex;
            flex-direction: column;
        }
        
        #file-list-container h2 {
            color: #333;
            margin-bottom: 10px;
            font-size: 18px;
            font-weight: 500;
            border-bottom: 2px solid #667eea;
            padding-bottom: 8px;
        }
        
        .path-bar {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 15px;
            padding: 8px 12px;
            background: rgba(102, 126, 234, 0.1);
            border-radius: 6px;
            font-family: 'Consolas', 'Monaco', monospace;
            font-size: 13px;
        }
        
        .back-btn {
            background: rgba(102, 126, 234, 0.8);
            color: white;
            border: none;
            padding: 6px 12px;
            border-radius: 4px;
            font-size: 12px;
            cursor: pointer;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            gap: 4px;
        }
        
        .back-btn:hover {
            background: rgba(102, 126, 234, 1);
            transform: translateX(-2px);
        }
        
        .back-btn:disabled {
            background: rgba(102, 126, 234, 0.3);
            cursor: not-allowed;
            transform: none;
        }
        
        .current-path {
            color: #667eea;
            font-weight: 500;
            flex: 1;
        }
        
        .status-indicator {
            padding: 5px 10px;
            border-radius: 15px;
            font-size: 12px;
            font-weight: 500;
            margin-bottom: 10px;
        }
        
        .status-connected {
            background: rgba(76, 175, 80, 0.2);
            color: #4CAF50;
        }
        
        .status-disconnected {
            background: rgba(244, 67, 54, 0.2);
            color: #f44336;
        }
        
        #file-list {
            flex: 1;
            overflow-y: auto;
            scrollbar-width: thin;
            scrollbar-color: #667eea transparent;
        }
        
        #file-list::-webkit-scrollbar {
            width: 6px;
        }
        
        #file-list::-webkit-scrollbar-track {
            background: transparent;
        }
        
        #file-list::-webkit-scrollbar-thumb {
            background: #667eea;
            border-radius: 3px;
        }
        
        #files {
            list-style: none;
        }
        
        #files li {
            padding: 10px 12px;
            margin: 5px 0;
            background: rgba(102, 126, 234, 0.1);
            border-radius: 6px;
            color: #333;
            cursor: pointer;
            transition: all 0.3s ease;
            word-break: break-all;
            font-family: 'Consolas', 'Monaco', monospace;
            font-size: 13px;
        }
        
        #files li:hover {
            background: rgba(102, 126, 234, 0.2);
            transform: translateX(5px);
        }
        
        .file-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            gap: 8px;
            position: relative;
        }
        
        .file-info {
            display: flex;
            align-items: center;
            gap: 8px;
            flex: 1;
            min-width: 0;
        }
        
        .file-icon {
            font-size: 16px;
            flex-shrink: 0;
        }
        
        .file-name {
            flex: 1;
            min-width: 0;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        
        .file-actions {
            display: none;
            gap: 5px;
            flex-shrink: 0;
        }
        
        #files li:hover .file-actions {
            display: flex;
        }
        
        .action-btn {
            background: rgba(102, 126, 234, 0.8);
            color: white;
            border: none;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 11px;
            cursor: pointer;
            transition: all 0.2s ease;
            white-space: nowrap;
        }
        
        .action-btn:hover {
            background: rgba(102, 126, 234, 1);
            transform: translateY(-1px);
        }
        
        .action-btn.delete-btn {
            background: rgba(244, 67, 54, 0.8);
        }
        
        .action-btn.delete-btn:hover {
            background: rgba(244, 67, 54, 1);
        }
        
        .modal {
            display: none;
            position: fixed;
            z-index: 1000;
            left: 0;
            top: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0, 0, 0, 0.5);
        }
        
        .modal-content {
            background-color: white;
            margin: 15% auto;
            padding: 20px;
            border-radius: 12px;
            width: 300px;
            text-align: center;
            box-shadow: 0 15px 35px rgba(0, 0, 0, 0.3);
        }
        
        .modal-buttons {
            margin-top: 20px;
            display: flex;
            gap: 10px;
            justify-content: center;
        }
        
        .modal-btn {
            padding: 8px 16px;
            border: none;
            border-radius: 6px;
            cursor: pointer;
            font-size: 14px;
        }
        
        .modal-btn.confirm {
            background: #f44336;
            color: white;
        }
        
        .modal-btn.cancel {
            background: #e0e0e0;
            color: #333;
        }
        
        #upload-container {
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(10px);
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 15px 35px rgba(0, 0, 0, 0.1);
            border: 1px solid rgba(255, 255, 255, 0.2);
        }
        
        #upload-container h3 {
            color: #333;
            margin-bottom: 15px;
            font-size: 16px;
            font-weight: 500;
        }
        
        #upload-form {
            display: flex;
            flex-direction: column;
            gap: 15px;
        }
        
        #file-input {
            padding: 10px;
            border: 2px dashed #667eea;
            border-radius: 8px;
            background: rgba(102, 126, 234, 0.05);
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        #file-input:hover {
            border-color: #764ba2;
            background: rgba(118, 75, 162, 0.1);
        }
        
        button {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 12px 20px;
            border-radius: 8px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.3s ease;
            box-shadow: 0 5px 15px rgba(102, 126, 234, 0.3);
        }
        
        button:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 25px rgba(102, 126, 234, 0.4);
        }
        
        button:active {
            transform: translateY(0);
        }
        
        @media (max-width: 768px) {
            #container {
                flex-direction: column;
                padding: 10px;
            }
            
            #sidebar {
                min-width: auto;
                flex: none;
                height: 300px;
            }
            
            #file-list-container {
                flex: 1;
            }
        }
    </style>
</head>
<body>
    <div id="header">
        <h1>WebShell Terminal</h1>
    </div>
    <div id="container">
        <div id="terminal-wrapper"></div>
        <div id="sidebar">
            <div id="file-list-container">
                <h2>📁 File Browser</h2>
                <div class="path-bar">
                    <button class="back-btn" id="backBtn" onclick="goBack()">
                        ⬅️ 返回
                    </button>
                    <span class="current-path" id="currentPath">/tmp</span>
                    <button class="back-btn" onclick="refreshFileList()">
                        🔄 刷新
                    </button>
                </div>
                <div class="status-indicator status-disconnected" id="connection-status">Disconnected</div>
                <div id="file-list">
                    <ul id="files"></ul>
                </div>
            </div>
            <div id="upload-container">
                <h3>📤 Upload File</h3>
                <form id="upload-form" enctype="multipart/form-data">
                    <input type="file" id="file-input" name="file" required>
                    <button type="submit">Upload to Current Directory</button>
                </form>
            </div>
        </div>
    </div>
    
    <!-- 删除确认模态框 -->
    <div id="deleteModal" class="modal">
        <div class="modal-content">
            <h3>确认删除</h3>
            <p id="deleteMessage">确定要删除这个文件吗？</p>
            <div class="modal-buttons">
                <button class="modal-btn confirm" onclick="confirmDelete()">删除</button>
                <button class="modal-btn cancel" onclick="closeDeleteModal()">取消</button>
            </div>
        </div>
    </div>
    
    <script src="https://cdn.jsdelivr.net/npm/xterm@4.14.1/lib/xterm.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/xterm-addon-fit@0.5.0/lib/xterm-addon-fit.js"></script>
    <script>
        // 终端初始化
        var term = new Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: 'Consolas, Monaco, monospace',
            convertEol: true,
            theme: {
                background: '#1e1e1e',
                foreground: '#ffffff',
                cursor: '#ffffff',
                cursorAccent: '#000000',
                selection: 'rgba(255, 255, 255, 0.3)',
                black: '#000000',
                red: '#e06c75',
                green: '#98c379',
                yellow: '#d19a66',
                blue: '#61afef',
                magenta: '#c678dd',
                cyan: '#56b6c2',
                white: '#abb2bf',
                brightBlack: '#5c6370',
                brightRed: '#e06c75',
                brightGreen: '#98c379',
                brightYellow: '#d19a66',
                brightBlue: '#61afef',
                brightMagenta: '#c678dd',
                brightCyan: '#56b6c2',
                brightWhite: '#ffffff'
            }
        });
        
        var fitAddon = new FitAddon.FitAddon();
        term.loadAddon(fitAddon);
        term.open(document.getElementById('terminal-wrapper'));
        
        // WebSocket 连接
        var statusIndicator = document.getElementById('connection-status');
        var protocol = (location.protocol === 'https:') ? 'wss://' : 'ws://';
        var socketUrl = protocol + window.location.host + '/ws';
        var socket = new WebSocket(socketUrl);

        socket.onmessage = function(event) {
            term.write(event.data);
        };
        
        socket.onopen = function() {
            term.write("Connected to WebShell Terminal.\r\n");
            statusIndicator.textContent = 'Connected';
            statusIndicator.className = 'status-indicator status-connected';
            setTimeout(function() { fitAddon.fit(); }, 100);
        };
        
        socket.onclose = function() {
            term.write("Disconnected from WebShell Terminal.\r\n");
            statusIndicator.textContent = 'Disconnected';
            statusIndicator.className = 'status-indicator status-disconnected';
        };

        term.onData(function(data) {
            if (socket.readyState === WebSocket.OPEN) {
                socket.send(data);
            }
        });

        // 文件浏览器状态
        var fileToDelete = '';
        var currentPath = '/tmp';
        
        // 文件图标映射
        function getFileIcon(filename, isDirectory) {
            if (isDirectory) return '📁';
            
            var ext = filename.split('.').pop().toLowerCase();
            var icons = {
                'txt': '📄', 'log': '📄', 'js': '📜', 'py': '🐍', 'go': '🔷',
                'java': '☕', 'cpp': '🔧', 'c': '🔧', 'html': '🌐', 'css': '🎨',
                'json': '📋', 'xml': '📋', 'zip': '📦', 'tar': '📦', 'gz': '📦',
                'pdf': '📕', 'doc': '📘', 'docx': '📘', 'xls': '📗', 'xlsx': '📗',
                'jpg': '🖼️', 'jpeg': '🖼️', 'png': '🖼️', 'gif': '🖼️',
                'mp4': '🎬', 'mp3': '🎵', 'wav': '🎵',
                'pem': '🔐', 'key': '🔐', 'cert': '🔐'
            };
            
            if (filename.startsWith('.')) return '🔸';
            return icons[ext] || '📄';
        }
        
        // 更新路径显示
        function updatePathDisplay() {
            var currentPathElement = document.getElementById('currentPath');
            if (currentPathElement) {
                currentPathElement.textContent = currentPath;
            }
            
            var backBtn = document.getElementById('backBtn');
            if (backBtn) {
                backBtn.disabled = currentPath === '/tmp';
            }
        }
        
        // 进入目录
        function enterDirectory(dirname) {
            if (currentPath.endsWith('/')) {
                currentPath = currentPath + dirname;
            } else {
                currentPath = currentPath + '/' + dirname;
            }
            updatePathDisplay();
            updateFileList();
        }
        
        // 返回上级目录
        function goBack() {
            if (currentPath === '/tmp') return;
            
            var pathParts = currentPath.split('/');
            pathParts.pop();
            currentPath = pathParts.join('/') || '/tmp';
            
            if (!currentPath.startsWith('/tmp')) {
                currentPath = '/tmp';
            }
            
            updatePathDisplay();
            updateFileList();
        }
        
        // 刷新文件列表
        function refreshFileList() {
            updateFileList();
        }
        
        // 复制文件路径
        function copyPath(filename) {
            var path = currentPath + (currentPath.endsWith('/') ? '' : '/') + filename;
            navigator.clipboard.writeText(path).then(function() {
                term.write('\r\n✅ Path copied: ' + path + '\r\n');
            }).catch(function(err) {
                term.write('\r\n❌ Failed to copy path\r\n');
            });
        }
        
        // 删除文件模态框
        function showDeleteModal(filename) {
            fileToDelete = filename;
            document.getElementById('deleteMessage').textContent = '确定要删除文件 "' + filename + '" 吗？';
            document.getElementById('deleteModal').style.display = 'block';
        }
        
        function closeDeleteModal() {
            document.getElementById('deleteModal').style.display = 'none';
            fileToDelete = '';
        }
        
        function confirmDelete() {
            if (!fileToDelete) return;
            
            fetch('/delete', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    filename: fileToDelete,
                    path: currentPath
                })
            })
            .then(response => response.text())
            .then(result => {
                term.write('\r\n🗑️ ' + result + '\r\n');
                updateFileList();
                closeDeleteModal();
            })
            .catch(error => {
                console.error('Error:', error);
                term.write('\r\n❌ Error deleting file\r\n');
                closeDeleteModal();
            });
        }

        // 更新文件列表
        function updateFileList() {
            var url = '/files?path=' + encodeURIComponent(currentPath);
            var fileList = document.getElementById('files');
            fileList.innerHTML = '<li style="color: #666; font-style: italic;">Loading...</li>';
            
            fetch(url)
            .then(response => {
                if (!response.ok) {
                    throw new Error('HTTP ' + response.status + ': ' + response.statusText);
                }
                return response.json();
            })
            .then(data => renderFileList(data))
            .catch(error => {
                console.error('Error:', error);
                fileList.innerHTML = '<li style="color: #f44336;">Error loading file list: ' + error.message + '</li>';
                term.write('\r\n❌ Error loading file list: ' + error.message + '\r\n');
            });
        }
        
        // 渲染文件列表
        function renderFileList(data) {
            var fileList = document.getElementById('files');
            fileList.innerHTML = '';
            
            if (!data.files || data.files.length === 0) {
                var li = document.createElement('li');
                li.innerHTML = '<div class="file-item"><div class="file-info"><span class="file-icon">📭</span><span class="file-name">Empty directory</span></div></div>';
                li.style.fontStyle = 'italic';
                li.style.color = '#666';
                fileList.appendChild(li);
                return;
            }
            
            data.files.forEach(function(item) {
                var li = document.createElement('li');
                var icon = getFileIcon(item.name, item.isDirectory);
                
                li.innerHTML = 
                    '<div class="file-item">' +
                        '<div class="file-info" data-filename="' + item.name + '" data-is-directory="' + item.isDirectory + '">' +
                            '<span class="file-icon">' + icon + '</span>' +
                            '<span class="file-name" title="' + item.name + '">' + item.name + '</span>' +
                        '</div>' +
                        '<div class="file-actions">' +
                            '<button class="action-btn copy-btn" data-filename="' + item.name + '">📋 复制路径</button>' +
                            (item.isDirectory ? '' : '<button class="action-btn delete-btn" data-filename="' + item.name + '">🗑️ 删除</button>') +
                        '</div>' +
                    '</div>';
                
                fileList.appendChild(li);
            });
            
            bindFileListEvents();
        }
        
        // 绑定文件列表事件
        function bindFileListEvents() {
            // 文件/文件夹点击事件
            document.querySelectorAll('.file-info').forEach(function(fileInfo) {
                fileInfo.addEventListener('click', function() {
                    var filename = this.getAttribute('data-filename');
                    var isDirectory = this.getAttribute('data-is-directory') === 'true';
                    
                    if (isDirectory) {
                        enterDirectory(filename);
                    } else {
                        if (socket.readyState === WebSocket.OPEN) {
                            var fullPath = currentPath + (currentPath.endsWith('/') ? '' : '/') + filename;
                            socket.send('ls -la "' + fullPath + '"\r');
                        }
                    }
                });
            });
            
            // 复制按钮事件
            document.querySelectorAll('.copy-btn').forEach(function(btn) {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    var filename = this.getAttribute('data-filename');
                    copyPath(filename);
                });
            });
            
            // 删除按钮事件
            document.querySelectorAll('.delete-btn').forEach(function(btn) {
                btn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    var filename = this.getAttribute('data-filename');
                    showDeleteModal(filename);
                });
            });
        }

        // 文件上传
        document.getElementById('upload-form').addEventListener('submit', function(e) {
            e.preventDefault();
            var formData = new FormData(this);
            var fileInput = document.getElementById('file-input');
            
            if (!fileInput.files[0]) {
                term.write('\r\nPlease select a file first.\r\n');
                return;
            }
            
            formData.append('path', currentPath);
            term.write('\r\nUploading file to ' + currentPath + '...\r\n');
            
            fetch('/upload', {
                method: 'POST',
                body: formData
            })
            .then(response => response.text())
            .then(result => {
                term.write('Upload result: ' + result + '\r\n');
                updateFileList();
                fileInput.value = '';
            })
            .catch(error => {
                console.error('Error:', error);
                term.write('Error uploading file.\r\n');
            });
        });

        // 窗口大小调整
        window.addEventListener('resize', function() {
            fitAddon.fit();
        });
        
        // 页面加载完成后调整终端大小
        window.addEventListener('load', function() {
            setTimeout(function() { fitAddon.fit(); }, 200);
        });
        
        // 模态框事件
        window.addEventListener('click', function(event) {
            var modal = document.getElementById('deleteModal');
            if (event.target === modal) {
                closeDeleteModal();
            }
        });
        
        document.addEventListener('keydown', function(event) {
            if (event.key === 'Escape') {
                closeDeleteModal();
            }
        });

        // 初始化
        setTimeout(function() { fitAddon.fit(); }, 100);
        updateFileList();
        updatePathDisplay();
        
        // 定期刷新文件列表
        setInterval(updateFileList, 30000);
    </script>
</body>
</html>`

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 文件信息结构体
type FileInfo struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
}

// 文件列表响应结构体
type FileListResponse struct {
	Files []FileInfo `json:"files"`
	Path  string     `json:"path"`
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

	// 创建shell进程
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

	var wg sync.WaitGroup
	wg.Add(2)

	// 从pty读取数据并发送到WebSocket
	go func() {
		defer wg.Done()
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
	go func() {
		defer wg.Done()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Error reading from websocket: %v", err)
				return
			}

			_, err = ptmx.Write(message)
			if err != nil {
				log.Printf("Error writing to pty: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}

// 文件上传处理器
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 获取目标路径
	targetPath := r.FormValue("path")
	if targetPath == "" {
		targetPath = "/tmp"
	}

	// 安全检查：确保路径在/tmp目录内
	cleanPath := filepath.Clean(targetPath)
	if !strings.HasPrefix(cleanPath, "/tmp") {
		cleanPath = "/tmp"
	}

	// 确保目标目录存在
	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		http.Error(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 创建目标文件
	dstPath := filepath.Join(cleanPath, header.Filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// 复制文件内容
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "File %s uploaded to %s successfully", header.Filename, cleanPath)
}

// 文件删除处理器
func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
		Path     string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	// 构建完整路径
	var fullPath string
	if req.Path != "" {
		cleanPath := filepath.Clean(req.Path)
		if !strings.HasPrefix(cleanPath, "/tmp") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		fullPath = filepath.Join(cleanPath, req.Filename)
	} else {
		fullPath = filepath.Join("/tmp", req.Filename)
	}

	// 安全检查
	if !strings.HasPrefix(fullPath, "/tmp") {
		http.Error(w, "Invalid file path", http.StatusBadRequest)
		return
	}

	err := os.Remove(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	fmt.Fprintf(w, "File %s deleted successfully", req.Filename)
}

// 文件列表处理器
func filesHandler(w http.ResponseWriter, r *http.Request) {
	requestPath := r.URL.Query().Get("path")
	if requestPath == "" {
		requestPath = "/tmp"
	}

	// 安全检查和路径清理
	cleanPath := filepath.Clean(requestPath)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join("/tmp", cleanPath)
	}
	
	if !strings.HasPrefix(cleanPath, "/tmp") {
		cleanPath = "/tmp"
	}

	// 检查目录是否存在且为目录
	stat, err := os.Stat(cleanPath)
	if os.IsNotExist(err) {
		http.Error(w, "Directory not found", http.StatusNotFound)
		return
	}
	
	if !stat.IsDir() {
		http.Error(w, "Path is not a directory", http.StatusBadRequest)
		return
	}

	// 读取目录内容
	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 构建文件信息列表
	var fileInfos []FileInfo
	for _, entry := range entries {
		fileInfos = append(fileInfos, FileInfo{
			Name:        entry.Name(),
			IsDirectory: entry.IsDir(),
		})
	}

	// 排序：目录优先，然后按名称排序
	sort.Slice(fileInfos, func(i, j int) bool {
		if fileInfos[i].IsDirectory != fileInfos[j].IsDirectory {
			return fileInfos[i].IsDirectory
		}
		return fileInfos[i].Name < fileInfos[j].Name
	})

	response := FileListResponse{
		Files: fileInfos,
		Path:  cleanPath,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

// 创建测试目录结构
func createTestDirectories() {
	testDirs := []string{
		"/tmp/documents",
		"/tmp/scripts",
		"/tmp/logs",
	}

	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Failed to create directory %s: %v", dir, err)
		}
	}

	// 创建示例文件
	testFiles := map[string]string{
		"/tmp/documents/readme.txt": "WebShell File Browser Demo\n\nThis is a demonstration file for the WebShell file browser functionality.",
		"/tmp/documents/example.md": "# WebShell Documentation\n\n## Features\n- Terminal access\n- File browser\n- File upload/download\n- File management",
		"/tmp/scripts/hello.sh":     "#!/bin/bash\necho \"Hello from WebShell!\"\ndate\n",
		"/tmp/logs/app.log":         fmt.Sprintf("Application started at %s\nWebShell initialized successfully\n", time.Now().Format(time.RFC3339)),
	}

	for filePath, content := range testFiles {
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			log.Printf("Failed to create file %s: %v", filePath, err)
		}
	}

	log.Println("Test directory structure created in /tmp")
}

func main() {
	// 创建测试目录结构
	createTestDirectories()

	// 设置信号处理
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// 创建HTTP路由
	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/ws", websocketHandler)
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/files", filesHandler)
	mux.HandleFunc("/delete", deleteHandler)

	// 创建服务器
	server := &http.Server{
		Addr:    ":5000",
		Handler: mux,
	}

	// 启动服务器
	go func() {
		fmt.Println("🚀 WebShell server starting on http://localhost:5000")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// 等待中断信号
	<-c
	fmt.Println("\n⏹️  Shutting down server...")

	// 优雅关闭
	if err := server.Shutdown(nil); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	fmt.Println("✅ Server stopped gracefully")
}