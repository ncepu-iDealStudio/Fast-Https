import asyncio
import websockets

# 处理接收到的消息
async def echo(websocket, path):
    async for message in websocket:
        # 将接收到的消息发送回客户端
        await websocket.send(message)

# 启动 WebSocket 服务器
start_server = websockets.serve(echo, "localhost", 7070)

# 运行事件循环
asyncio.get_event_loop().run_until_complete(start_server)
asyncio.get_event_loop().run_forever()
