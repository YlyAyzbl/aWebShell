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
from aiohttp import web

# 简易 HTML
HTML_PAGE = r"""<!DOCTYPE html>
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
        os.execlp('/var/jb/usr/bin/bash', 'bash')
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

app = web.Application()
app.router.add_get('/', index)
app.router.add_get('/ws', websocket_handler)

if __name__ == '__main__':
    web.run_app(app, host='0.0.0.0', port=5000)
