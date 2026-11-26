"""Loopback-only standalone CPA browser fixture when Docker ports are not browser reachable.
No real credentials, production configuration, or third-party packages.
"""
import http.server
import urllib.request
import urllib.error

class Proxy(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        req=urllib.request.Request('http://127.0.0.1:18319'+self.path,headers={k:v for k,v in self.headers.items() if k.lower() not in ('host','connection')})
        try:
            response=urllib.request.urlopen(req,timeout=20)
        except urllib.error.HTTPError as exc:
            response=exc
        with response:
            self.send_response(response.status)
            for k,v in response.headers.items():
                if k.lower() not in ('transfer-encoding','connection','content-length'):
                    self.send_header(k,v)
            body=response.read()
            self.send_header('Content-Length',str(len(body)))
            self.end_headers()
            self.wfile.write(body)

if __name__=='__main__':
    http.server.ThreadingHTTPServer(('127.0.0.1',18082),Proxy).serve_forever()
