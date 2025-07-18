#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
基于 aiohttp + pty.fork() 的简易 WebShell，
可正确使用 screen，且在切换 screen 窗口后 Ctrl+C 依然有效。
"""

import asyncio
import os
import pty
import sys
import json
import datetime
from pathlib import Path
from aiohttp import web
from aiohttp.web_fileresponse import FileResponse
from aiohttp.web_request import Request

# 完整的 HTML 界面 (与 Go 版本保持一致)
HTML_PAGE = r"""<!DOCTYPE html>
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
</html>
"""

async def index(request):
    return web.Response(text=HTML_PAGE, content_type="text/html")

async def websocket_handler(request):
    ws = web.WebSocketResponse()
    await ws.prepare(request)

    loop = asyncio.get_running_loop()

    # 1) fork 出子进程 + pty
    pid, master_fd = pty.fork()

    if pid == 0:
        # 子进程: 直接执行 /bin/bash （screen 里你也可以安装后再执行）
        os.execlp('/bin/bash', 'bash')
        sys.exit(0)
    else:
        # 父进程：异步读 master_fd，并通过 websocket 发给前端
        async def read_pty():
            while True:
                try:
                    data = await loop.run_in_executor(None, os.read, master_fd, 1024)
                    if not data:
                        break
                    await ws.send_str(data.decode(errors='ignore'))
                except Exception as e:
                    print("read pty error:", e)
                    break

        read_task = asyncio.create_task(read_pty())

        # 接收前端按键数据，写入 pty
        async for msg in ws:
            if msg.type == web.WSMsgType.TEXT:
                # 不要手动 kill，而是直接将 \x03 原封不动地写到 pty
                os.write(master_fd, msg.data.encode())

        read_task.cancel()

        # 当 ws 断开，结束子进程
        try:
            os.kill(pid, 15)  # SIGTERM
        except ProcessLookupError:
            pass

        return ws

# 文件浏览器处理器
async def files_handler(request):
    request_path = request.query.get('path', '/tmp')
    
    # 安全检查和路径清理
    clean_path = os.path.normpath(request_path)
    if not os.path.isabs(clean_path):
        clean_path = os.path.join('/tmp', clean_path)
    
    if not clean_path.startswith('/tmp'):
        clean_path = '/tmp'
    
    try:
        # 检查目录是否存在
        if not os.path.exists(clean_path):
            return web.json_response({'error': 'Directory not found'}, status=404)
        
        if not os.path.isdir(clean_path):
            return web.json_response({'error': 'Path is not a directory'}, status=400)
        
        # 读取目录内容
        entries = os.listdir(clean_path)
        file_infos = []
        
        for entry in entries:
            full_path = os.path.join(clean_path, entry)
            is_directory = os.path.isdir(full_path)
            file_infos.append({
                'name': entry,
                'isDirectory': is_directory
            })
        
        # 排序：目录优先，然后按名称排序
        file_infos.sort(key=lambda x: (not x['isDirectory'], x['name']))
        
        response = {
            'files': file_infos,
            'path': clean_path
        }
        
        return web.json_response(response, headers={'Cache-Control': 'no-cache'})
        
    except Exception as e:
        return web.json_response({'error': str(e)}, status=500)

# 文件上传处理器
async def upload_handler(request):
    try:
        reader = await request.multipart()
        
        file_data = None
        target_path = '/tmp'
        filename = None
        
        # 先收集所有字段
        async for field in reader:
            if field.name == 'file':
                filename = field.filename
                if not filename:
                    return web.Response(text='No filename provided', status=400)
                
                # 读取文件内容到内存
                file_data = await field.read()
                
            elif field.name == 'path':
                target_path = await field.text()
        
        if not file_data:
            return web.Response(text='No file uploaded', status=400)
        
        # 安全检查：确保路径在/tmp目录内
        clean_path = os.path.normpath(target_path)
        if not clean_path.startswith('/tmp'):
            clean_path = '/tmp'
        
        # 确保目标目录存在
        os.makedirs(clean_path, exist_ok=True)
        
        # 创建目标文件路径
        dst_path = os.path.join(clean_path, filename)
        
        # 写入文件
        with open(dst_path, 'wb') as f:
            f.write(file_data)
        
        return web.Response(text=f'File {filename} uploaded to {clean_path} successfully')
        
    except Exception as e:
        return web.Response(text=f'Upload failed: {str(e)}', status=500)

# 文件删除处理器
async def delete_handler(request):
    try:
        data = await request.json()
        filename = data.get('filename')
        file_path = data.get('path', '/tmp')
        
        if not filename:
            return web.Response(text='Filename is required', status=400)
        
        # 构建完整路径
        clean_path = os.path.normpath(file_path)
        if not clean_path.startswith('/tmp'):
            clean_path = '/tmp'
        
        full_path = os.path.join(clean_path, filename)
        
        # 安全检查
        if not full_path.startswith('/tmp'):
            return web.Response(text='Invalid file path', status=400)
        
        # 删除文件
        if os.path.exists(full_path):
            os.remove(full_path)
            return web.Response(text=f'File {filename} deleted successfully')
        else:
            return web.Response(text='File not found', status=404)
            
    except Exception as e:
        return web.Response(text=f'Delete failed: {str(e)}', status=500)

# 创建测试目录结构
def create_test_directories():
    test_dirs = [
        '/tmp/documents',
        '/tmp/scripts',
        '/tmp/logs',
    ]
    
    for dir_path in test_dirs:
        os.makedirs(dir_path, exist_ok=True)
    
    # 创建示例文件
    test_files = {
        '/tmp/documents/readme.txt': 'WebShell File Browser Demo\n\nThis is a demonstration file for the WebShell file browser functionality.',
        '/tmp/documents/example.md': '# WebShell Documentation\n\n## Features\n- Terminal access\n- File browser\n- File upload/download\n- File management',
        '/tmp/scripts/hello.sh': '#!/bin/bash\necho "Hello from WebShell!"\ndate\n',
        '/tmp/logs/app.log': f'Application started at {datetime.datetime.now().isoformat()}\nWebShell initialized successfully\n',
    }
    
    for file_path, content in test_files.items():
        try:
            with open(file_path, 'w') as f:
                f.write(content)
        except Exception as e:
            print(f"Failed to create file {file_path}: {e}")
    
    print("Test directory structure created in /tmp")

app = web.Application()
app.router.add_get('/', index)
app.router.add_get('/ws', websocket_handler)
app.router.add_get('/files', files_handler)
app.router.add_post('/upload', upload_handler)
app.router.add_post('/delete', delete_handler)

if __name__ == '__main__':
    # 创建测试目录结构
    create_test_directories()
    
    print("🚀 WebShell server starting on http://localhost:5000")
    web.run_app(app, host='0.0.0.0', port=5000)
