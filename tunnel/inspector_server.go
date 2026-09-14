package tunnel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

const inspectorHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PortShare Inspector</title>
    <style>
        :root {
            --bg: #1e1e1e;
            --surface: #2d2d2d;
            --text: #ffffff;
            --text-muted: #a0a0a0;
            --border: #3d3d3d;
            --primary: #3b82f6;
            --success: #22c55e;
            --error: #ef4444;
            --warning: #f59e0b;
        }
        body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); display: flex; height: 100vh; overflow: hidden; }
        .sidebar { width: 350px; border-right: 1px solid var(--border); display: flex; flex-direction: column; background: var(--bg); }
        .header { padding: 15px; font-weight: bold; font-size: 1.2rem; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; }
        .req-list { flex: 1; overflow-y: auto; }
        .req-item { padding: 12px 15px; border-bottom: 1px solid var(--border); cursor: pointer; display: flex; flex-direction: column; gap: 4px; }
        .req-item:hover { background: var(--surface); }
        .req-item.active { background: var(--surface); border-left: 3px solid var(--primary); }
        .req-row { display: flex; justify-content: space-between; align-items: center; }
        .method { font-weight: bold; font-size: 0.9rem; }
        .path { font-size: 0.9rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 200px; color: var(--text-muted); }
        .status { font-size: 0.85rem; padding: 2px 6px; border-radius: 4px; font-weight: bold; }
        .s-200 { background: rgba(34, 197, 94, 0.2); color: var(--success); }
        .s-300 { background: rgba(245, 158, 11, 0.2); color: var(--warning); }
        .s-400, .s-500 { background: rgba(239, 68, 68, 0.2); color: var(--error); }
        .time { font-size: 0.8rem; color: var(--text-muted); }
        .main { flex: 1; display: flex; flex-direction: column; background: var(--bg); overflow: hidden; }
        .detail-header { padding: 15px 20px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; background: var(--surface); }
        .btn { background: var(--primary); color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; font-weight: bold; }
        .btn:hover { opacity: 0.9; }
        .detail-body { flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 20px; }
        .section { background: var(--surface); border-radius: 8px; border: 1px solid var(--border); overflow: hidden; }
        .section-title { padding: 10px 15px; background: rgba(255,255,255,0.05); font-weight: bold; border-bottom: 1px solid var(--border); }
        .section-content { padding: 15px; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; font-size: 0.85rem; line-height: 1.5; white-space: pre-wrap; overflow-x: auto; color: #e5e7eb; }
        .key-val { display: flex; gap: 10px; margin-bottom: 4px; }
        .key { color: var(--text-muted); width: 150px; flex-shrink: 0; }
        .val { word-break: break-all; }
        .empty-state { display: flex; justify-content: center; align-items: center; height: 100%; color: var(--text-muted); }
        .tabs { display: flex; border-bottom: 1px solid var(--border); }
        .tab { padding: 10px 20px; cursor: pointer; border-bottom: 2px solid transparent; }
        .tab.active { border-bottom-color: var(--primary); color: var(--primary); }
    </style>
</head>
<body>
    <div class="sidebar">
        <div class="header">Requests <span id="req-count" style="font-size: 0.8rem; font-weight: normal; color: var(--text-muted);">0</span></div>
        <div class="req-list" id="req-list"></div>
    </div>
    <div class="main" id="main">
        <div class="empty-state">Select a request to view details</div>
    </div>

    <script>
        let requests = [];
        let selectedId = null;

        function getStatusClass(status) {
            if (status >= 200 && status < 300) return 's-200';
            if (status >= 300 && status < 400) return 's-300';
            return 's-400';
        }

        function renderList() {
            const list = document.getElementById('req-list');
            document.getElementById('req-count').innerText = requests.length;
            list.innerHTML = requests.map(r => {
                const active = r.id === selectedId ? 'active' : '';
                const sClass = getStatusClass(r.respStatus);
                const status = r.respStatus || '---';
                return '<div class="req-item ' + active + '" onclick="selectReq(\'' + r.id + '\')">' +
                    '<div class="req-row">' +
                        '<span class="method">' + r.method + '</span>' +
                        '<span class="status ' + sClass + '">' + status + '</span>' +
                    '</div>' +
                    '<div class="req-row">' +
                        '<span class="path" title="' + r.path + '">' + r.path + '</span>' +
                        '<span class="time">' + r.durationMs + 'ms</span>' +
                    '</div>' +
                '</div>';
            }).join('');
        }

        function selectReq(id) {
            selectedId = id;
            renderList();
            renderDetail();
        }

        function formatHeaders(headers) {
            if (!headers) return 'No headers';
            return Object.entries(headers).map(([k, v]) => 
                '<div class="key-val"><div class="key">' + k + '</div><div class="val">' + v.join(', ') + '</div></div>'
            ).join('');
        }

        function formatBody(base64Body) {
            if (!base64Body) return 'No body';
            try {
                const text = atob(base64Body);
                try {
                    return JSON.stringify(JSON.parse(text), null, 2);
                } catch {
                    return text; // fallback to raw string
                }
            } catch(e) {
                return '[Binary or Unreadable Data]';
            }
        }

        async function replay(id) {
            await fetch('/api/replay/' + id, { method: 'POST' });
        }

        function renderDetail() {
            const main = document.getElementById('main');
            const r = requests.find(x => x.id === selectedId);
            if (!r) return;

            let reqBodyHtml = '';
            if (r.reqBody) {
                reqBodyHtml = '<div class="section"><div class="section-title">Request Body</div><div class="section-content">' + formatBody(r.reqBody) + '</div></div>';
            }

            let respBodyHtml = '';
            if (r.respBody) {
                respBodyHtml = '<div class="section"><div class="section-title">Response Body</div><div class="section-content">' + formatBody(r.respBody) + '</div></div>';
            }

            main.innerHTML = '<div class="detail-header">' +
                    '<div>' +
                        '<span class="method" style="margin-right: 10px">' + r.method + '</span>' +
                        '<span>' + r.path + '</span>' +
                    '</div>' +
                    '<button class="btn" onclick="replay(\'' + r.id + '\')">Replay</button>' +
                '</div>' +
                '<div class="detail-body">' +
                    '<div class="section">' +
                        '<div class="section-title">Request Headers</div>' +
                        '<div class="section-content">' + formatHeaders(r.reqHeaders) + '</div>' +
                    '</div>' +
                    reqBodyHtml +
                    '<div class="section">' +
                        '<div class="section-title">Response Headers</div>' +
                        '<div class="section-content">' + formatHeaders(r.respHeaders) + '</div>' +
                    '</div>' +
                    respBodyHtml +
                '</div>';
        }

        async function fetchRequests() {
            const res = await fetch('/api/requests');
            const data = await res.json();
            // preserve selection if it still exists
            requests = data.reverse(); 
            renderList();
            if (selectedId) renderDetail();
        }

        setInterval(fetchRequests, 1000);
        fetchRequests();
    </script>
</body>
</html>
`

func StartInspector(port int) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(inspectorHTML))
	})

	mux.HandleFunc("/api/requests", func(w http.ResponseWriter, r *http.Request) {
		reqs := GetCapturedRequests()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reqs)
	})

	mux.HandleFunc("/api/replay/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Path[len("/api/replay/"):]
		reqRec := GetRequest(id)
		if reqRec == nil {
			http.Error(w, "Request not found", http.StatusNotFound)
			return
		}

		port, do := GetReplayTarget()
		if do == nil {
			http.Error(w, "Replay target not configured", http.StatusInternalServerError)
			return
		}

		go func() {
			var bodyReader io.Reader
			if len(reqRec.ReqBody) > 0 {
				bodyReader = bytes.NewReader(reqRec.ReqBody)
			}
			urlStr := fmt.Sprintf("http://127.0.0.1:%d%s", port, reqRec.Path)
			newReq, err := http.NewRequest(reqRec.Method, urlStr, bodyReader)
			if err != nil {
				return
			}
			for k, v := range reqRec.ReqHeaders {
				newReq.Header[k] = v
			}
			_, _ = Intercept(newReq, do)
		}()
		
		w.WriteHeader(http.StatusOK)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("Inspector UI available at http://%s\n", addr)
	_ = http.ListenAndServe(addr, mux)
}
