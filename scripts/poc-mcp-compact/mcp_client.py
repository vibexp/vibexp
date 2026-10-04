import json, sys, requests
TOKEN = open("/tmp/vibexp-poc-token").read().strip()
class MCP:
    def __init__(self, path, origin="http://localhost:18080"):
        self.url = origin + path; self.sid = None; self.n = 0
        self.rpc("initialize", {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "poc", "version": "0"}})
        self.rpc("notifications/initialized", None, notify=True)
    def rpc(self, method, params, notify=False):
        self.n += 1
        body = {"jsonrpc": "2.0", "method": method}
        if params is not None: body["params"] = params
        if not notify: body["id"] = self.n
        h = {"Authorization": "Bearer " + TOKEN, "Accept": "application/json, text/event-stream", "Content-Type": "application/json"}
        if self.sid: h["Mcp-Session-Id"] = self.sid
        r = requests.post(self.url, headers=h, json=body)
        self.sid = r.headers.get("Mcp-Session-Id", self.sid)
        if notify: return None
        r.raise_for_status()
        text = r.text
        if "text/event-stream" in r.headers.get("content-type", ""):
            text = [l[5:].strip() for l in text.splitlines() if l.startswith("data:")][-1]
        out = json.loads(text)
        if "error" in out: raise RuntimeError(out["error"])
        return out["result"]
    def tools(self): return self.rpc("tools/list", {})["tools"]
    def call(self, name, args): return self.rpc("tools/call", {"name": name, "arguments": args})
def text(res): return res["content"][0]["text"]
