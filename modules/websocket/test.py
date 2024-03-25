import asyncio
import websockets

# 处理接收到的消息
async def echo(websocket, path):
    async for message in websocket:
        # 将接收到的消息发送回客户端
        await websocket.send(message)
        # 每隔3秒向客户端推送一个 "hello" 消息
        while True:
            await asyncio.sleep(3)
            await websocket.send("hello")

# 设置超时时间为10秒
ping_interval = 30
ping_timeout = 5

# 启动 WebSocket 服务器，并设置超时时间
start_server = websockets.serve(
    echo, "localhost", 7070, ping_interval=ping_interval, ping_timeout=ping_timeout
)

# 运行事件循环
asyncio.get_event_loop().run_until_complete(start_server)

print("正在启动websocket服务 ... ")
asyncio.get_event_loop().run_forever()
