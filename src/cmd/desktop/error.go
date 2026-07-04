package main

import (
	"fmt"
	"time"

	"github.com/webview/webview_go"
)

// @sk-task kvn-desktop#T3.1: show error page with start button (AC-006, AC-012)
func showErrorPage(w webview.WebView, svc *ServiceManager, serverURL string) {
	w.SetHtml(errorPageHTML())

	w.Bind("startService", func() {
		if err := svc.Start(); err != nil {
			w.Eval(fmt.Sprintf(
				`document.getElementById('status').innerText = 'Ошибка: %s'`,
				escapeJS(err.Error()),
			))
			return
		}
		w.Eval(`document.getElementById('status').innerText = 'Служба запускается...'`)
		go func() {
			for i := 0; i < 30; i++ {
				time.Sleep(1 * time.Second)
				if checkServer(serverURL) {
					w.Dispatch(func() {
						w.Navigate(serverURL)
					})
					return
				}
			}
			w.Dispatch(func() {
				w.Eval(`document.getElementById('status').innerText = 'Служба не отвечает после запуска'`)
			})
		}()
	})
}

func escapeJS(s string) string {
	escaped := ""
	for _, c := range s {
		switch c {
		case '\'':
			escaped += "\\'"
		case '"':
			escaped += "\\\""
		case '\\':
			escaped += "\\\\"
		case '\n':
			escaped += "\\n"
		case '\r':
			escaped += "\\r"
		default:
			escaped += string(c)
		}
	}
	return escaped
}

func errorPageHTML() string {
	return `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
  body {
    margin: 0;
    font-family: 'Segoe UI', system-ui, sans-serif;
    background: #161616;
    color: #d0d0d0;
    display: flex;
    justify-content: center;
    align-items: center;
    height: 100vh;
  }
  .container { text-align: center; padding: 40px; }
  h1 { font-size: 24px; margin-bottom: 8px; color: #d0d0d0; font-weight: 600; }
  p { font-size: 14px; margin-bottom: 24px; color: #888; }
  button {
    background: #1a5a9e;
    color: #fff;
    border: none;
    padding: 10px 28px;
    border-radius: 4px;
    font-size: 14px;
    cursor: pointer;
    font-weight: 600;
    transition: background 0.15s;
  }
  button:hover { background: #1e6ab8; }
  #status { margin-top: 16px; font-size: 13px; color: #888; }
</style>
</head>
<body>
<div class="container">
  <h1>Служба kvn-web не запущена</h1>
  <p>Запустите службу, чтобы продолжить работу с KVN Desktop.</p>
  <button onclick="window.startService()">Запустить службу</button>
  <div id="status"></div>
</div>
</body>
</html>`
}
